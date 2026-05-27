package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegister(t *testing.T) {
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
