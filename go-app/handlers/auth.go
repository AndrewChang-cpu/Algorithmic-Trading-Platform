package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"application-server/db"
	"application-server/middleware"
	"application-server/models"
	"application-server/queue"

	"golang.org/x/crypto/bcrypt"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, models.ErrorResponse{Error: msg})
}

// checkRateLimit increments a Redis counter for the given key and returns an
// error if the count exceeds max within window. Fails open on Redis errors so
// that legitimate requests are not blocked when Redis is unavailable.
func checkRateLimit(ctx context.Context, key string, max int64, window time.Duration) error {
	rdb := queue.GetClient()
	pipe := rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("rate limit check failed (failing open) for key %s: %v", key, err)
		return nil
	}
	if incr.Val() > max {
		return fmt.Errorf("rate limit exceeded")
	}
	return nil
}

// clientIP returns the host portion of RemoteAddr, or the raw RemoteAddr if
// it cannot be split.
func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// setRefreshCookie sets the refresh_token httpOnly cookie on the response.
func setRefreshCookie(w http.ResponseWriter, token string) {
	cookie := &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/auth",
		MaxAge:   86400,
	}
	if os.Getenv("APP_ENV") == "production" {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

// clearRefreshCookie clears the refresh_token cookie.
func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/auth",
		MaxAge:   0,
	})
}

// Register handles POST /api/auth/register
func Register(w http.ResponseWriter, r *http.Request) {
	if err := checkRateLimit(r.Context(), "ratelimit:register:"+clientIP(r), 5, time.Minute); err != nil {
		writeError(w, http.StatusTooManyRequests, "too many requests")
		return
	}

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusUnprocessableEntity, "password must be at least 8 characters")
		return
	}

	// Check email uniqueness
	var exists bool
	err := db.Pool.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", req.Email,
	).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hashing password")
		return
	}

	var userID string
	err = db.Pool.QueryRow(r.Context(),
		"INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id",
		req.Email, string(hash),
	).Scan(&userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating user")
		return
	}

	accessToken, refreshToken, err := issueTokens(r, userID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	setRefreshCookie(w, refreshToken)
	writeJSON(w, http.StatusCreated, map[string]string{
		"userId":      userID,
		"accessToken": accessToken,
	})
}

// Login handles POST /api/auth/login
func Login(w http.ResponseWriter, r *http.Request) {
	if err := checkRateLimit(r.Context(), "ratelimit:login:"+clientIP(r), 10, time.Minute); err != nil {
		writeError(w, http.StatusTooManyRequests, "too many requests")
		return
	}

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var userID, passwordHash string
	err := db.Pool.QueryRow(r.Context(),
		"SELECT id, password_hash FROM users WHERE email=$1", req.Email,
	).Scan(&userID, &passwordHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	accessToken, refreshToken, err := issueTokens(r, userID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	setRefreshCookie(w, refreshToken)
	writeJSON(w, http.StatusOK, models.AuthResponse{
		AccessToken: accessToken,
		UserID:      userID,
	})
}

// Refresh handles POST /api/auth/refresh
func Refresh(w http.ResponseWriter, r *http.Request) {
	if err := checkRateLimit(r.Context(), "ratelimit:refresh:"+clientIP(r), 20, time.Minute); err != nil {
		writeError(w, http.StatusTooManyRequests, "too many requests")
		return
	}

	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	tokenHash := middleware.HashToken(cookie.Value)

	var userID, email string
	err = db.Pool.QueryRow(r.Context(), `
		SELECT rt.user_id, u.email
		FROM refresh_tokens rt
		JOIN users u ON rt.user_id = u.id
		WHERE rt.token_hash=$1 AND rt.expires_at > NOW()
	`, tokenHash).Scan(&userID, &email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Rotate: delete old token
	if _, err := db.Pool.Exec(r.Context(), "DELETE FROM refresh_tokens WHERE token_hash=$1", tokenHash); err != nil {
		log.Printf("refresh token delete failed: %v", err)
		writeError(w, http.StatusInternalServerError, "token rotation failed")
		return
	}

	accessToken, refreshToken, err := issueTokens(r, userID, email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	setRefreshCookie(w, refreshToken)
	writeJSON(w, http.StatusOK, models.AuthResponse{
		AccessToken: accessToken,
	})
}

// Logout handles POST /api/auth/logout
func Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		// No cookie present — clear and return 204
		clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	tokenHash := middleware.HashToken(cookie.Value)
	db.Pool.Exec(r.Context(), "DELETE FROM refresh_tokens WHERE token_hash=$1", tokenHash)
	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// issueTokens generates and stores a new access+refresh token pair.
func issueTokens(r *http.Request, userID, email string) (string, string, error) {
	accessToken, err := middleware.GenerateAccessToken(userID, email)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := middleware.GenerateRefreshToken()
	if err != nil {
		return "", "", err
	}
	tokenHash := middleware.HashToken(refreshToken)
	_, err = db.Pool.Exec(r.Context(), `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, time.Now().Add(24*time.Hour))
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}
