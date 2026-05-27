package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"application-server/db"
	"application-server/middleware"
	"application-server/models"

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

// Register handles POST /api/auth/register
func Register(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusCreated, map[string]string{
		"userId":       userID,
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
	})
}

// Login handles POST /api/auth/login
func Login(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// Refresh handles POST /api/auth/refresh
func Refresh(w http.ResponseWriter, r *http.Request) {
	var req models.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tokenHash := middleware.HashToken(req.RefreshToken)

	var userID, email string
	err := db.Pool.QueryRow(r.Context(), `
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
	db.Pool.Exec(r.Context(), "DELETE FROM refresh_tokens WHERE token_hash=$1", tokenHash)

	accessToken, refreshToken, err := issueTokens(r, userID, email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// Logout handles POST /api/auth/logout
func Logout(w http.ResponseWriter, r *http.Request) {
	var req models.LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	tokenHash := middleware.HashToken(req.RefreshToken)
	db.Pool.Exec(r.Context(), "DELETE FROM refresh_tokens WHERE token_hash=$1", tokenHash)
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
	`, userID, tokenHash, time.Now().Add(7*24*time.Hour))
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}
