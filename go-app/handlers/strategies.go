package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"application-server/db"
	"application-server/middleware"
	"application-server/models"
	s3client "application-server/s3"

	"github.com/jackc/pgx/v5"
)

// validateWithPythonService calls the Python validation service to check strategy source.
// Returns (className, "", nil) on success, ("", violation, nil) on violation, or ("", "", err) on service error.
func validateWithPythonService(ctx context.Context, source string) (className string, violation string, err error) {
	serviceURL := os.Getenv("PYTHON_SERVICE_URL")
	if serviceURL == "" {
		return "", "", fmt.Errorf("PYTHON_SERVICE_URL not set")
	}
	body, _ := json.Marshal(map[string]string{"source": source})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceURL+"/validate", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("python service returned %d", resp.StatusCode)
	}
	var result struct {
		Valid     bool   `json:"valid"`
		ClassName string `json:"class_name"`
		Violation string `json:"violation"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	if !result.Valid {
		return "", result.Violation, nil
	}
	return result.ClassName, "", nil
}

// ListStrategies handles GET /api/strategies
func ListStrategies(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := db.Pool.Query(r.Context(), `
		SELECT s.id, s.name, COALESCE(s.description, ''), s.created_at,
		       COALESCE(MAX(sv.version_number), 0) AS latest_version,
		       COUNT(DISTINCT j.id) AS run_count,
		       MAX(pm.sharpe_ratio) AS best_sharpe
		FROM strategies s
		LEFT JOIN strategy_versions sv ON sv.strategy_id = s.id
		LEFT JOIN jobs j ON j.strategy_version_id = sv.id
		LEFT JOIN performance_metrics pm ON pm.job_id = j.id
		WHERE s.user_id = $1
		GROUP BY s.id
		ORDER BY s.created_at DESC
	`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	var strategies []models.StrategyResponse
	for rows.Next() {
		var s models.StrategyResponse
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.CreatedAt, &s.LatestVersion, &s.RunCount, &s.BestSharpe); err != nil {
			log.Printf("scan error: %v", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		strategies = append(strategies, s)
	}
	if err := rows.Err(); err != nil {
		log.Printf("rows error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if strategies == nil {
		strategies = []models.StrategyResponse{}
	}
	writeJSON(w, http.StatusOK, strategies)
}

// UploadStrategy handles POST /api/strategies
func UploadStrategy(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	name := r.FormValue("name")
	if name == "" {
		writeError(w, http.StatusUnprocessableEntity, "name is required")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	source := string(data)

	className, violation, err := validateWithPythonService(r.Context(), source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "validation service error")
		return
	}
	if violation != "" {
		writeError(w, http.StatusUnprocessableEntity, violation)
		return
	}

	var strategyID string
	err = db.Pool.QueryRow(r.Context(),
		"INSERT INTO strategies (user_id, name) VALUES ($1, $2) RETURNING id",
		userID, name,
	).Scan(&strategyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating strategy")
		return
	}

	s3Key := fmt.Sprintf("%s/%s/v1/main.py", userID, strategyID)

	var versionID string
	err = db.Pool.QueryRow(r.Context(),
		"INSERT INTO strategy_versions (strategy_id, version_number, s3_key) VALUES ($1, 1, $2) RETURNING id",
		strategyID, s3Key,
	).Scan(&versionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating version")
		return
	}

	if err := s3client.PutObject(r.Context(), s3Key, data, "text/x-python"); err != nil {
		db.Pool.Exec(r.Context(), "DELETE FROM strategy_versions WHERE id=$1", versionID) //nolint:errcheck
		writeError(w, http.StatusInternalServerError, "failed to upload strategy")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"strategyId":    strategyID,
		"versionId":     versionID,
		"versionNumber": 1,
		"className":     className,
	})
}

// GetStrategy handles GET /api/strategies/:id
func GetStrategy(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	strategyID := r.PathValue("id")

	var s models.StrategyResponse
	var ownerID string
	err := db.Pool.QueryRow(r.Context(), `
		SELECT s.id, s.name, COALESCE(s.description, ''), s.created_at, s.user_id
		FROM strategies s
		WHERE s.id = $1
	`, strategyID).Scan(&s.ID, &s.Name, &s.Description, &s.CreatedAt, &ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "strategy not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := db.Pool.Query(r.Context(),
		"SELECT id, version_number, created_at FROM strategy_versions WHERE strategy_id=$1 ORDER BY version_number DESC",
		strategyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var v models.StrategyVersionResponse
		if err := rows.Scan(&v.ID, &v.VersionNumber, &v.CreatedAt); err != nil {
			log.Printf("scan error: %v", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		s.Versions = append(s.Versions, v)
		if v.VersionNumber > s.LatestVersion {
			s.LatestVersion = v.VersionNumber
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("rows error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	var stats models.StrategyStats
	if err := db.Pool.QueryRow(r.Context(), `
		SELECT COUNT(j.id), MAX(pm.sharpe_ratio), AVG(pm.total_return_pct)
		FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		LEFT JOIN performance_metrics pm ON pm.job_id = j.id
		WHERE sv.strategy_id = $1
	`, strategyID).Scan(&stats.RunCount, &stats.BestSharpe, &stats.AvgReturn); err != nil {
		log.Printf("GetStrategy: stats query error for strategy %s: %v", strategyID, err)
		// continue — partial response with zero stats is acceptable
	}
	s.Stats = &stats

	writeJSON(w, http.StatusOK, s)
}

// UploadNewVersion handles POST /api/strategies/:id/versions
func UploadNewVersion(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	strategyID := r.PathValue("id")

	var ownerID string
	err := db.Pool.QueryRow(r.Context(), "SELECT user_id FROM strategies WHERE id=$1", strategyID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "strategy not found")
		} else {
			writeError(w, http.StatusInternalServerError, "database error")
		}
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	source := string(data)

	_, violation, err := validateWithPythonService(r.Context(), source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "validation service error")
		return
	}
	if violation != "" {
		writeError(w, http.StatusUnprocessableEntity, violation)
		return
	}

	var nextVersion int
	if err := db.Pool.QueryRow(r.Context(),
		"SELECT COALESCE(MAX(version_number),0)+1 FROM strategy_versions WHERE strategy_id=$1",
		strategyID).Scan(&nextVersion); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	s3Key := fmt.Sprintf("%s/%s/v%d/main.py", userID, strategyID, nextVersion)

	var versionID string
	if err := db.Pool.QueryRow(r.Context(),
		"INSERT INTO strategy_versions (strategy_id, version_number, s3_key) VALUES ($1,$2,$3) RETURNING id",
		strategyID, nextVersion, s3Key,
	).Scan(&versionID); err != nil {
		writeError(w, http.StatusInternalServerError, "creating version")
		return
	}

	if err := s3client.PutObject(r.Context(), s3Key, data, "text/x-python"); err != nil {
		db.Pool.Exec(r.Context(), "DELETE FROM strategy_versions WHERE id=$1", versionID) //nolint:errcheck
		writeError(w, http.StatusInternalServerError, "failed to upload strategy")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"versionId":     versionID,
		"versionNumber": nextVersion,
	})
}

// GetVersionCode handles GET /api/strategies/:id/versions/:versionId/code
func GetVersionCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	strategyID := r.PathValue("id")
	versionID := r.PathValue("versionId")

	var ownerID string
	if err := db.Pool.QueryRow(r.Context(), "SELECT user_id FROM strategies WHERE id=$1", strategyID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "strategy not found")
		} else {
			writeError(w, http.StatusInternalServerError, "database error")
		}
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var s3Key string
	if err := db.Pool.QueryRow(r.Context(),
		"SELECT s3_key FROM strategy_versions WHERE id=$1 AND strategy_id=$2",
		versionID, strategyID).Scan(&s3Key); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "version not found")
		} else {
			writeError(w, http.StatusInternalServerError, "database error")
		}
		return
	}

	data, err := s3client.GetObject(r.Context(), s3Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "fetching code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": string(data)})
}

// DeleteStrategy handles DELETE /api/strategies/:id
func DeleteStrategy(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	strategyID := r.PathValue("id")

	var ownerID string
	if err := db.Pool.QueryRow(r.Context(), "SELECT user_id FROM strategies WHERE id=$1", strategyID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "strategy not found")
		} else {
			writeError(w, http.StatusInternalServerError, "database error")
		}
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	s3client.DeletePrefix(r.Context(), fmt.Sprintf("%s/%s/", userID, strategyID))
	db.Pool.Exec(r.Context(), "DELETE FROM strategies WHERE id=$1", strategyID)
	w.WriteHeader(http.StatusNoContent)
}
