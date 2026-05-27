package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func setupTestKeys(t *testing.T) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key gen: %v", err)
	}
	SetKeysForTest(priv, &priv.PublicKey)
}

func makeToken(t *testing.T, userID, email string, exp time.Duration) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"exp":   time.Now().Add(exp).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(privateKey)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	return signed
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r.Context())
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(userID))
}

func TestRequireAuth_ValidToken(t *testing.T) {
	setupTestKeys(t)
	token := makeToken(t, "user-123", "test@test.com", 15*time.Minute)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "user-123" {
		t.Errorf("expected user-123 in body, got %s", rr.Body.String())
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	setupTestKeys(t)
	token := makeToken(t, "user-123", "test@test.com", -1*time.Minute)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	setupTestKeys(t)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRequireAuth_TamperedToken(t *testing.T) {
	setupTestKeys(t)
	token := makeToken(t, "user-123", "test@test.com", 15*time.Minute) + "tampered"

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := HashToken("mytoken")
	h2 := HashToken("mytoken")
	if h1 != h2 {
		t.Error("HashToken not deterministic")
	}
	if HashToken("a") == HashToken("b") {
		t.Error("HashToken collision for different inputs")
	}
}
