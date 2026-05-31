package handlers

import (
	"context"
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
		if resp["refreshToken"] != "" {
			t.Error("refreshToken must not appear in response body")
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
		if resp["refreshToken"] != "" {
			t.Error("refreshToken must not appear in response body")
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

// loginGetCookie registers (if needed) and logs in, returning the refresh_token cookie value.
func loginGetCookie(t *testing.T, email, password string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	Login(rr, jsonReq("POST", "/", `{"email":"`+email+`","password":"`+password+`"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rr.Code, rr.Body.String())
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == "refresh_token" {
			return c.Value
		}
	}
	t.Fatal("no refresh_token cookie in login response")
	return ""
}

// refreshWithCookie sends a Refresh request with the given cookie value.
func refreshWithCookie(cookieValue string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/", nil)
	if cookieValue != "" {
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: cookieValue})
	}
	rr := httptest.NewRecorder()
	Refresh(rr, req)
	return rr
}

func TestLoginSetsCookie(t *testing.T) {
	testRedis.FlushAll()
	Register(httptest.NewRecorder(), jsonReq("POST", "/",
		`{"email":"cookie-test@test.com","password":"password123"}`))

	rr := httptest.NewRecorder()
	Login(rr, jsonReq("POST", "/", `{"email":"cookie-test@test.com","password":"password123"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	setCookie := rr.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "refresh_token=") {
		t.Errorf("Set-Cookie header missing refresh_token: %s", setCookie)
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Errorf("Set-Cookie header missing HttpOnly: %s", setCookie)
	}
}

func TestRefreshReadsCookie(t *testing.T) {
	testRedis.FlushAll()
	Register(httptest.NewRecorder(), jsonReq("POST", "/",
		`{"email":"refresh-cookie@test.com","password":"password123"}`))
	cookieValue := loginGetCookie(t, "refresh-cookie@test.com", "password123")

	t.Run("200 with valid cookie", func(t *testing.T) {
		rr := refreshWithCookie(cookieValue)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]string
		decodeJSON(t, rr, &resp)
		if resp["accessToken"] == "" {
			t.Error("expected new accessToken in response")
		}
		// New refresh cookie should be set
		setCookie := rr.Header().Get("Set-Cookie")
		if !strings.Contains(setCookie, "refresh_token=") {
			t.Errorf("expected new refresh_token cookie, got: %s", setCookie)
		}
	})

	t.Run("401 with no cookie", func(t *testing.T) {
		rr := refreshWithCookie("")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 with no cookie, got %d", rr.Code)
		}
	})
}

func TestRefresh(t *testing.T) {
	testRedis.FlushAll()
	t.Run("200 returns new tokens", func(t *testing.T) {
		Register(httptest.NewRecorder(), jsonReq("POST", "/",
			`{"email":"refresh-200@test.com","password":"password123"}`))
		cookieValue := loginGetCookie(t, "refresh-200@test.com", "password123")

		rr := refreshWithCookie(cookieValue)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
		var newTokens map[string]string
		decodeJSON(t, rr, &newTokens)
		if newTokens["accessToken"] == "" {
			t.Error("expected new accessToken")
		}
	})

	t.Run("401 after token rotation (old token rejected)", func(t *testing.T) {
		Register(httptest.NewRecorder(), jsonReq("POST", "/",
			`{"email":"refresh-rotate@test.com","password":"password123"}`))
		cookieValue := loginGetCookie(t, "refresh-rotate@test.com", "password123")

		// First use consumes the token.
		refreshWithCookie(cookieValue)

		// Same token must now be rejected.
		rr := refreshWithCookie(cookieValue)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 after rotation, got %d", rr.Code)
		}
	})

	t.Run("401 on invalid token", func(t *testing.T) {
		rr := refreshWithCookie("invalid-token-not-in-db")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})
}

func TestLogoutClearsCookie(t *testing.T) {
	testRedis.FlushAll()
	Register(httptest.NewRecorder(), jsonReq("POST", "/",
		`{"email":"logout-cookie@test.com","password":"password123"}`))
	cookieValue := loginGetCookie(t, "logout-cookie@test.com", "password123")

	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: cookieValue})
	rr := httptest.NewRecorder()
	Logout(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	setCookie := rr.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "refresh_token=;") {
		t.Errorf("expected cleared refresh_token cookie, got: %s", setCookie)
	}
}

func TestLogout(t *testing.T) {
	testRedis.FlushAll()
	Register(httptest.NewRecorder(), jsonReq("POST", "/",
		`{"email":"logout-test@test.com","password":"password123"}`))
	cookieValue := loginGetCookie(t, "logout-test@test.com", "password123")

	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: cookieValue})
	rr := httptest.NewRecorder()
	Logout(rr, req)
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

func TestRefreshRateLimit(t *testing.T) {
	testRedis.FlushAll()

	ip := "10.3.0.1"
	// Send 21 refresh requests from the same IP with no cookie.
	// Rate limit is checked before cookie validation, so the first 20 get 401
	// (no cookie) and the 21st gets 429 (rate limited).
	for i := 1; i <= 21; i++ {
		req := httptest.NewRequest("POST", "/", nil)
		req.Header.Set("X-Forwarded-For", ip)
		rr := httptest.NewRecorder()
		Refresh(rr, req)
		if i < 21 {
			if rr.Code == http.StatusTooManyRequests {
				t.Fatalf("attempt %d unexpectedly rate-limited (got 429)", i)
			}
		} else {
			if rr.Code != http.StatusTooManyRequests {
				t.Fatalf("expected 429 on attempt 21, got %d: %s", rr.Code, rr.Body.String())
			}
		}
	}
}

func TestClientIP_UsesRemoteAddr(t *testing.T) {
	// XFF header must be ignored; only RemoteAddr is used.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	got := clientIP(req)
	if got != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1 (from RemoteAddr), got %s", got)
	}

	// No port in RemoteAddr: raw value returned.
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.2"
	got2 := clientIP(req2)
	if got2 != "10.0.0.2" {
		t.Errorf("expected 10.0.0.2, got %s", got2)
	}
}

func TestRefreshDeleteFailure(t *testing.T) {
	testRedis.FlushAll()
	// Register a user to get a valid refresh token via cookie.
	Register(httptest.NewRecorder(), jsonReqWithIP("POST", "/",
		`{"email":"refresh-delfail@test.com","password":"password123"}`, "10.2.0.1"))
	cookieValue := loginGetCookie(t, "refresh-delfail@test.com", "password123")

	// Simulate DELETE failure by installing a BEFORE DELETE trigger that raises
	// an exception. SELECT still succeeds (the token row is visible), but DELETE
	// is blocked — triggering the 500 path in Refresh.
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `
		CREATE FUNCTION _block_refresh_delete()
		  RETURNS trigger LANGUAGE plpgsql AS $$
		  BEGIN RAISE EXCEPTION 'delete blocked by test fixture'; END;
		$$`); err != nil {
		t.Fatalf("create block function: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
		CREATE TRIGGER _block_delete
		  BEFORE DELETE ON refresh_tokens
		  FOR EACH ROW EXECUTE FUNCTION _block_refresh_delete()`); err != nil {
		t.Fatalf("create block trigger: %v", err)
	}
	defer func() {
		testPool.Exec(ctx, "DROP TRIGGER IF EXISTS _block_delete ON refresh_tokens")  //nolint:errcheck
		testPool.Exec(ctx, "DROP FUNCTION IF EXISTS _block_refresh_delete()")          //nolint:errcheck
	}()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: cookieValue})
	Refresh(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when DELETE fails, got %d: %s", rr.Code, rr.Body.String())
	}
}

