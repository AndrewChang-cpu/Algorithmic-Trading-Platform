package handlers

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"application-server/db"
	"application-server/middleware"
	"application-server/models"
	s3client "application-server/s3"
)

// blockedImports is the set of Python modules/builtins disallowed in strategies.
var blockedImports = []string{"os", "subprocess", "socket", "sys", "shutil", "pathlib", "eval", "exec", "__import__", "compile"}

// validatePythonStrategy performs a simple string-scan check on Python source.
// Returns (className, "") on success, or ("", "violation message") on violation.
func validatePythonStrategy(source string) (className string, violation string) {
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, blocked := range blockedImports {
			if strings.HasPrefix(trimmed, "import "+blocked) ||
				strings.HasPrefix(trimmed, "from "+blocked) {
				return "", fmt.Sprintf("import %s detected on line %d", blocked, i+1)
			}
			if blocked == "eval" || blocked == "exec" || blocked == "__import__" || blocked == "compile" {
				if strings.Contains(trimmed, blocked+"(") {
					return "", fmt.Sprintf("%s call detected on line %d", blocked, i+1)
				}
			}
		}
	}

	re := regexp.MustCompile(`class\s+(\w+)\s*\(\s*QCAlgorithm\s*\)`)
	matches := re.FindStringSubmatch(source)
	if matches == nil {
		return "", "no QCAlgorithm subclass found"
	}
	return matches[1], ""
}

// ListStrategies handles GET /api/strategies
func ListStrategies(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

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
			continue
		}
		strategies = append(strategies, s)
	}
	if strategies == nil {
		strategies = []models.StrategyResponse{}
	}
	writeJSON(w, http.StatusOK, strategies)
}

// UploadStrategy handles POST /api/strategies
func UploadStrategy(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

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
		writeError(w, http.StatusInternalServerError, "reading file")
		return
	}
	source := string(data)

	className, violation := validatePythonStrategy(source)
	if violation != "" {
		writeError(w, http.StatusUnprocessableEntity, "AST violation: "+violation)
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
	if err := s3client.PutObject(r.Context(), s3Key, data, "text/x-python"); err != nil {
		db.Pool.Exec(r.Context(), "DELETE FROM strategies WHERE id=$1", strategyID)
		writeError(w, http.StatusInternalServerError, "uploading file")
		return
	}

	var versionID string
	err = db.Pool.QueryRow(r.Context(),
		"INSERT INTO strategy_versions (strategy_id, version_number, s3_key) VALUES ($1, 1, $2) RETURNING id",
		strategyID, s3Key,
	).Scan(&versionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating version")
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
	userID := middleware.GetUserID(r.Context())
	strategyID := r.PathValue("id")

	var s models.StrategyResponse
	err := db.Pool.QueryRow(r.Context(), `
		SELECT s.id, s.name, COALESCE(s.description, ''), s.created_at
		FROM strategies s
		WHERE s.id = $1
	`, strategyID).Scan(&s.ID, &s.Name, &s.Description, &s.CreatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "strategy not found")
		return
	}

	var ownerID string
	db.Pool.QueryRow(r.Context(), "SELECT user_id FROM strategies WHERE id=$1", strategyID).Scan(&ownerID)
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, _ := db.Pool.Query(r.Context(),
		"SELECT id, version_number, created_at FROM strategy_versions WHERE strategy_id=$1 ORDER BY version_number DESC",
		strategyID)
	defer rows.Close()
	for rows.Next() {
		var v models.StrategyVersionResponse
		rows.Scan(&v.ID, &v.VersionNumber, &v.CreatedAt)
		s.Versions = append(s.Versions, v)
		if v.VersionNumber > s.LatestVersion {
			s.LatestVersion = v.VersionNumber
		}
	}

	var stats models.StrategyStats
	db.Pool.QueryRow(r.Context(), `
		SELECT COUNT(j.id), MAX(pm.sharpe_ratio), AVG(pm.total_return_pct)
		FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		LEFT JOIN performance_metrics pm ON pm.job_id = j.id
		WHERE sv.strategy_id = $1
	`, strategyID).Scan(&stats.RunCount, &stats.BestSharpe, &stats.AvgReturn)
	s.Stats = &stats

	writeJSON(w, http.StatusOK, s)
}

// UploadNewVersion handles POST /api/strategies/:id/versions
func UploadNewVersion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	strategyID := r.PathValue("id")

	var ownerID string
	db.Pool.QueryRow(r.Context(), "SELECT user_id FROM strategies WHERE id=$1", strategyID).Scan(&ownerID)
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
	data, _ := io.ReadAll(file)
	source := string(data)

	_, violation := validatePythonStrategy(source)
	if violation != "" {
		writeError(w, http.StatusUnprocessableEntity, "AST violation: "+violation)
		return
	}

	var nextVersion int
	db.Pool.QueryRow(r.Context(),
		"SELECT COALESCE(MAX(version_number),0)+1 FROM strategy_versions WHERE strategy_id=$1",
		strategyID).Scan(&nextVersion)

	s3Key := fmt.Sprintf("%s/%s/v%d/main.py", userID, strategyID, nextVersion)
	s3client.PutObject(r.Context(), s3Key, data, "text/x-python")

	var versionID string
	db.Pool.QueryRow(r.Context(),
		"INSERT INTO strategy_versions (strategy_id, version_number, s3_key) VALUES ($1,$2,$3) RETURNING id",
		strategyID, nextVersion, s3Key,
	).Scan(&versionID)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"versionId":     versionID,
		"versionNumber": nextVersion,
	})
}

// GetVersionCode handles GET /api/strategies/:id/versions/:versionId/code
func GetVersionCode(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	strategyID := r.PathValue("id")
	versionID := r.PathValue("versionId")

	var ownerID string
	db.Pool.QueryRow(r.Context(), "SELECT user_id FROM strategies WHERE id=$1", strategyID).Scan(&ownerID)
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var s3Key string
	db.Pool.QueryRow(r.Context(),
		"SELECT s3_key FROM strategy_versions WHERE id=$1 AND strategy_id=$2",
		versionID, strategyID).Scan(&s3Key)
	if s3Key == "" {
		writeError(w, http.StatusNotFound, "version not found")
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
	userID := middleware.GetUserID(r.Context())
	strategyID := r.PathValue("id")

	var ownerID string
	db.Pool.QueryRow(r.Context(), "SELECT user_id FROM strategies WHERE id=$1", strategyID).Scan(&ownerID)
	if ownerID == "" {
		writeError(w, http.StatusNotFound, "strategy not found")
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

