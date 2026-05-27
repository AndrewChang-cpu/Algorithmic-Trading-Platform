package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	userIDKey    contextKey = "userID"
	userEmailKey contextKey = "userEmail"
)

var (
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
)

// LoadKeys loads RS256 key pair from PEM files.
func LoadKeys(privatePath, publicPath string) error {
	privBytes, err := os.ReadFile(privatePath)
	if err != nil {
		return fmt.Errorf("reading private key: %w", err)
	}
	priv, err := jwt.ParseRSAPrivateKeyFromPEM(privBytes)
	if err != nil {
		return fmt.Errorf("parsing private key: %w", err)
	}

	pubBytes, err := os.ReadFile(publicPath)
	if err != nil {
		return fmt.Errorf("reading public key: %w", err)
	}
	pub, err := jwt.ParseRSAPublicKeyFromPEM(pubBytes)
	if err != nil {
		return fmt.Errorf("parsing public key: %w", err)
	}

	privateKey = priv
	publicKey = pub
	return nil
}

// SetKeysForTest allows tests to inject keys directly.
func SetKeysForTest(priv *rsa.PrivateKey, pub *rsa.PublicKey) {
	privateKey = priv
	publicKey = pub
}

// GenerateAccessToken creates a 15-minute RS256 JWT.
func GenerateAccessToken(userID, email string) (string, error) {
	if privateKey == nil {
		return "", errors.New("private key not loaded")
	}
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}

// GenerateRefreshToken returns a random 32-byte hex string.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex digest of token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// RequireAuth is HTTP middleware that validates the Bearer JWT.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicKey == nil {
			http.Error(w, `{"error":"server misconfigured"}`, http.StatusInternalServerError)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return publicKey, nil
		})

		if err != nil || !token.Valid {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
			return
		}

		userID, _ := claims["sub"].(string)
		email, _ := claims["email"].(string)

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		ctx = context.WithValue(ctx, userEmailKey, email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID extracts the authenticated user ID from context.
func GetUserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

// GetUserEmail extracts the authenticated user email from context.
func GetUserEmail(ctx context.Context) string {
	v, _ := ctx.Value(userEmailKey).(string)
	return v
}
