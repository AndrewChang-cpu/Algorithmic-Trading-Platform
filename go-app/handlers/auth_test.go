package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegister(t *testing.T) {
	testRedis.FlushAll()
	t.Run("201 on valid input", func(t *testing.T) {
		rr := httptest.NewRecorder()
		Register(rr, jsonReq("POST", "/", `{"email":"reg-201@test.com","password":"password123"}`))
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]string
		decodeJSON(t, rr, &resp)
		if resp["userId"] == "" {
			t.Error("expected userId in response")
		}
		if resp["accessToken"] == "" {
			t.Error("expected accessToken in response")
		}
	})

	t.Run("409 on duplicate email", func(t *testing.T) {
		// First registration succeeds.
		Register(httptest.NewRecorder(), jsonReq("POST", "/", `{"email":"reg-409@test.com","password":"password123"}`))
		// Second registration with the same email must fail.
		rr := httptest.NewRecorder()
		Register(rr, jsonReq("POST", "/", `{"email":"reg-409@test.com","password":"password123"}`))
		if rr.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rr.Code)
		}
	})

	t.Run("422 on short password", func(t *testing.T) {
		rr := httptest.NewRecorder()
		Register(rr, jsonReq("POST", "/", `{"email":"reg-422@test.com","password":"short"}`))
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", rr.Code)
		}
	})
}

func TestLogin(t *testing.T) {
	testRedis.FlushAll()
	// Seed a user for login tests.
	Register(httptest.NewRecorder(), jsonReq("POST", "/",
		`{"email":"login-test@test.com","password":"password123"}`))

	t.Run("200 on valid credentials", func(t *testing.T) {
		rr := httptest.NewRecorder()
		Login(rr, jsonReq("POST", "/", `{"email":"login-test@test.com","password":"password123"}`))
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]string
		decodeJSON(t, rr, &resp)
		if resp["accessToken"] == "" {
			t.Error("expected accessToken in response")
		}
	})

	t.Run("401 on wrong password", func(t *testing.T) {
		rr := httptest.NewRecorder()
		Login(rr, jsonReq("POST", "/", `{"email":"login-test@test.com","password":"wrongpassword"}`))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("401 on unknown email", func(t *testing.T) {
		rr := httptest.NewRecorder()
		Login(rr, jsonReq("POST", "/", `{"email":"nobody@test.com","password":"password123"}`))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})
}

func TestRefresh(t *testing.T) {
	testRedis.FlushAll()
	t.Run("200 returns new tokens", func(t *testing.T) {
		rr1 := httptest.NewRecorder()
		Register(rr1, jsonReq("POST", "/", `{"email":"refresh-200@test.com","password":"password123"}`))
		var tokens map[string]string
		decodeJSON(t, rr1, &tokens)

		rr2 := httptest.NewRecorder()
		Refresh(rr2, jsonReq("POST", "/", `{"refreshToken":"`+tokens["refreshToken"]+`"}`))
		if rr2.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr2.Code, rr2.Body.String())
		}
		var newTokens map[string]string
		decodeJSON(t, rr2, &newTokens)
		if newTokens["accessToken"] == "" {
			t.Error("expected new accessToken")
		}
		if newTokens["refreshToken"] == "" {
			t.Error("expected new refreshToken")
		}
	})

	t.Run("401 after token rotation (old token rejected)", func(t *testing.T) {
		rr1 := httptest.NewRecorder()
		Register(rr1, jsonReq("POST", "/", `{"email":"refresh-rotate@test.com","password":"password123"}`))
		var tokens map[string]string
		decodeJSON(t, rr1, &tokens)

		// First use consumes the token.
		Refresh(httptest.NewRecorder(), jsonReq("POST", "/", `{"refreshToken":"`+tokens["refreshToken"]+`"}`))

		// Same token must now be rejected.
		rr := httptest.NewRecorder()
		Refresh(rr, jsonReq("POST", "/", `{"refreshToken":"`+tokens["refreshToken"]+`"}`))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 after rotation, got %d", rr.Code)
		}
	})

	t.Run("401 on invalid token", func(t *testing.T) {
		rr := httptest.NewRecorder()
		Refresh(rr, jsonReq("POST", "/", `{"refreshToken":"invalid-token-not-in-db"}`))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})
}

func TestLogout(t *testing.T) {
	testRedis.FlushAll()
	rr1 := httptest.NewRecorder()
	Register(rr1, jsonReq("POST", "/", `{"email":"logout-test@test.com","password":"password123"}`))
	var tokens map[string]string
	json.NewDecoder(rr1.Body).Decode(&tokens)

	rr := httptest.NewRecorder()
	Logout(rr, jsonReq("POST", "/", `{"refreshToken":"`+tokens["refreshToken"]+`"}`))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}

// jsonReqWithIP builds a POST request with a JSON body and a spoofed client IP
// via X-Forwarded-For, so rate-limit tests can use distinct keys per subtest.
func jsonReqWithIP(method, target, body, ip string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	return req
}

func TestLoginRateLimit(t *testing.T) {
	testRedis.FlushAll()
	// Seed a user so the first 10 attempts can reach bcrypt comparison.
	Register(httptest.NewRecorder(), jsonReqWithIP("POST", "/",
		`{"email":"ratelimit-login@test.com","password":"password123"}`, "10.0.0.1"))

	ip := "10.1.0.1" // unique IP for this test to avoid state from other tests

	t.Run("first 10 attempts not rate-limited", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			rr := httptest.NewRecorder()
			Login(rr, jsonReqWithIP("POST", "/",
				`{"email":"ratelimit-login@test.com","password":"password123"}`, ip))
			if rr.Code == http.StatusTooManyRequests {
				t.Fatalf("attempt %d unexpectedly rate-limited (got 429)", i+1)
			}
		}
	})

	t.Run("11th attempt returns 429", func(t *testing.T) {
		rr := httptest.NewRecorder()
		Login(rr, jsonReqWithIP("POST", "/",
			`{"email":"ratelimit-login@test.com","password":"password123"}`, ip))
		if rr.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 on 11th attempt, got %d", rr.Code)
		}
	})
}

func TestRefreshDeleteFailure(t *testing.T) {
	// Register a user to get a valid refresh token.
	rr1 := httptest.NewRecorder()
	Register(rr1, jsonReqWithIP("POST", "/",
		`{"email":"refresh-delfail@test.com","password":"password123"}`, "10.2.0.1"))
	var tokens map[string]string
	decodeJSON(t, rr1, &tokens)

	// Simulate DELETE failure by replacing the refresh_tokens table with a
	// read-only view over a renamed backing table. SELECT still succeeds (the
	// view exposes the same rows), but DELETE against a plain view fails.
	ctx := context.Background()
	if _, err := testPool.Exec(ctx,
		"ALTER TABLE refresh_tokens RENAME TO refresh_tokens_bak"); err != nil {
		t.Fatalf("rename table: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		"CREATE VIEW refresh_tokens AS SELECT * FROM refresh_tokens_bak"); err != nil {
		// Restore and skip if the view cannot be created.
		testPool.Exec(ctx, "ALTER TABLE refresh_tokens_bak RENAME TO refresh_tokens") //nolint:errcheck
		t.Skipf("could not create view for DELETE failure simulation: %v", err)
	}
	defer func() {
		testPool.Exec(ctx, "DROP VIEW IF EXISTS refresh_tokens")                       //nolint:errcheck
		testPool.Exec(ctx, "ALTER TABLE refresh_tokens_bak RENAME TO refresh_tokens") //nolint:errcheck
	}()

	rr := httptest.NewRecorder()
	Refresh(rr, jsonReq("POST", "/", `{"refreshToken":"`+tokens["refreshToken"]+`"}`))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when DELETE fails, got %d: %s", rr.Code, rr.Body.String())
	}
}
