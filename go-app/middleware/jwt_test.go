package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
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

// makeTokenRawClaims signs a token with exactly the provided claims map,
// allowing tests to omit or empty specific fields.
func makeTokenRawClaims(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(privateKey)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	return signed
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := GetUserID(r.Context())
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

func TestRequireAuth_MissingSub(t *testing.T) {
	setupTestKeys(t)
	token := makeTokenRawClaims(t, jwt.MapClaims{
		"email": "test@test.com",
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing sub, got %d", rr.Code)
	}
}

func TestRequireAuth_EmptyEmail(t *testing.T) {
	setupTestKeys(t)
	token := makeTokenRawClaims(t, jwt.MapClaims{
		"sub":   "user-123",
		"email": "",
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty email, got %d", rr.Code)
	}
}

func TestGetUserID_PresentAndMissing(t *testing.T) {
	setupTestKeys(t)

	// Present: full valid token goes through RequireAuth, context should have userID
	token := makeToken(t, "user-456", "x@y.com", 15*time.Minute)
	var capturedID string
	var capturedOk bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID, capturedOk = GetUserID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	RequireAuth(handler).ServeHTTP(rr, req)
	if !capturedOk || capturedID != "user-456" {
		t.Errorf("expected (user-456, true), got (%q, %v)", capturedID, capturedOk)
	}

	// Missing: context without the key set
	id, ok := GetUserID(req.Context())
	if ok || id != "" {
		t.Errorf("expected ('', false) for bare context, got (%q, %v)", id, ok)
	}
}

func TestRequireAuth_ErrorBodyIsGeneric(t *testing.T) {
	setupTestKeys(t)
	// Use an expired token to trigger the ValidateToken error path
	token := makeToken(t, "user-123", "test@test.com", -1*time.Minute)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode body: %v (raw: %s)", err, rr.Body.String())
	}
	if body["error"] != "invalid or expired token" {
		t.Errorf("expected generic error, got: %q", body["error"])
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
