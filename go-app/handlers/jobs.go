package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"application-server/db"
	"application-server/middleware"
	"application-server/models"
	"application-server/queue"
)

// SubmitJob handles POST /api/jobs
func SubmitJob(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req models.SubmitJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Symbols) == 0 || req.Resolution == "" || req.StrategyVersionID == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "strategyVersionId, type, symbols, and resolution are required")
		return
	}

	// Verify strategy_version belongs to this user
	var ownerID string
	db.Pool.QueryRow(r.Context(), `
		SELECT s.user_id FROM strategy_versions sv
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE sv.id = $1
	`, req.StrategyVersionID).Scan(&ownerID)
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "strategy version not found or not owned by user")
		return
	}

	dataSource := req.DataSource
	if dataSource == "" {
		dataSource = "alpaca"
	}

	warmupDays := req.WarmupDays
	if warmupDays == 0 {
		warmupDays = 365
	}

	var jobID string
	err := db.Pool.QueryRow(r.Context(), `
		INSERT INTO jobs (user_id, strategy_version_id, type, status, data_source,
		                  symbols, resolution, start_date, end_date, warmup_days)
		VALUES ($1, $2, $3, 'queued', $4, $5, $6, $7::date, $8::date, $9)
		RETURNING id
	`, userID, req.StrategyVersionID, req.Type, dataSource,
		req.Symbols, req.Resolution,
		nullableDate(req.StartDate), nullableDate(req.EndDate), warmupDays,
	).Scan(&jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating job")
		return
	}

	if req.Type == "live" {
		queue.EnqueueLive(jobID)
	} else {
		queue.EnqueueBacktest(jobID)
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": jobID})
}

func nullableDate(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// GetJob handles GET /api/jobs/:id
func GetJob(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := r.PathValue("id")

	var j models.JobResponse
	var ownerID string
	err := db.Pool.QueryRow(r.Context(), `
		SELECT j.id, s.name, sv.version_number, j.type, j.status, j.data_source,
		       j.symbols, j.resolution, j.error_message, j.created_at, j.started_at, j.completed_at,
		       s.user_id
		FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE j.id = $1
	`, jobID).Scan(
		&j.ID, &j.StrategyName, &j.VersionNumber, &j.Type, &j.Status, &j.DataSource,
		&j.Symbols, &j.Resolution, &j.ErrorMessage, &j.CreatedAt, &j.StartedAt, &j.CompletedAt,
		&ownerID,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// ListJobs handles GET /api/jobs
func ListJobs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	q := r.URL.Query()

	pageStr := q.Get("page")
	if pageStr == "" {
		pageStr = "1"
	}
	limitStr := q.Get("limit")
	if limitStr == "" {
		limitStr = "20"
	}
	page, _ := strconv.Atoi(pageStr)
	limit, _ := strconv.Atoi(limitStr)
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	typeFilter := q.Get("type")
	statusFilter := q.Get("status")

	// filterArgs holds only the filter conditions (for the COUNT query).
	// listArgs extends filterArgs with limit and offset for the main query.
	filterArgs := []interface{}{userID}
	where := "WHERE j.user_id = $1"
	idx := 2
	if typeFilter != "" {
		where += fmt.Sprintf(" AND j.type = $%d", idx)
		filterArgs = append(filterArgs, typeFilter)
		idx++
	}
	if statusFilter != "" {
		where += fmt.Sprintf(" AND j.status = $%d", idx)
		filterArgs = append(filterArgs, statusFilter)
		idx++
	}

	// Append limit and offset as the last two params for the list query.
	listArgs := append(filterArgs, limit, offset) //nolint:gocritic
	limitIdx := idx
	offsetIdx := idx + 1

	rows, err := db.Pool.Query(r.Context(), fmt.Sprintf(`
		SELECT j.id, s.name, sv.version_number, j.type, j.status, j.data_source,
		       j.symbols, j.resolution, j.error_message, j.created_at, j.started_at, j.completed_at,
		       pm.total_return_pct, pm.sharpe_ratio
		FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		LEFT JOIN performance_metrics pm ON pm.job_id = j.id
		%s
		ORDER BY j.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, limitIdx, offsetIdx), listArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	var jobs []models.JobResponse
	for rows.Next() {
		var j models.JobResponse
		var returnPct, sharpe *float64
		rows.Scan(
			&j.ID, &j.StrategyName, &j.VersionNumber, &j.Type, &j.Status, &j.DataSource,
			&j.Symbols, &j.Resolution, &j.ErrorMessage, &j.CreatedAt, &j.StartedAt, &j.CompletedAt,
			&returnPct, &sharpe,
		)
		jobs = append(jobs, j)
	}

	var total int
	db.Pool.QueryRow(r.Context(), fmt.Sprintf(`
		SELECT COUNT(*) FROM jobs j %s
	`, where), filterArgs...).Scan(&total)

	if jobs == nil {
		jobs = []models.JobResponse{}
	}
	writeJSON(w, http.StatusOK, models.JobListResponse{Jobs: jobs, Total: total})
}

// GetJobMetrics handles GET /api/jobs/:id/metrics
func GetJobMetrics(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := r.PathValue("id")

	var ownerID string
	db.Pool.QueryRow(r.Context(), `
		SELECT s.user_id FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE j.id = $1
	`, jobID).Scan(&ownerID)
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	// Return all columns from performance_metrics as a map.
	rows, err := db.Pool.Query(r.Context(),
		"SELECT * FROM performance_metrics WHERE job_id = $1", jobID)
	if err != nil || !rows.Next() {
		writeError(w, http.StatusNotFound, "metrics not available yet")
		return
	}
	defer rows.Close()

	fieldDescriptions := rows.FieldDescriptions()
	vals, _ := rows.Values()
	result := make(map[string]interface{})
	for i, fd := range fieldDescriptions {
		result[string(fd.Name)] = vals[i]
	}
	writeJSON(w, http.StatusOK, result)
}

// GetPortfolio handles GET /api/jobs/:id/portfolio
func GetPortfolio(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := r.PathValue("id")

	var ownerID string
	db.Pool.QueryRow(r.Context(), `
		SELECT s.user_id FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE j.id = $1
	`, jobID).Scan(&ownerID)
	if ownerID != userID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	qp := r.URL.Query()
	from := qp.Get("from")
	to := qp.Get("to")
	if from == "" {
		from = time.Now().AddDate(-1, 0, 0).Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().Format(time.RFC3339)
	}

	rows, err := db.Pool.Query(r.Context(), `
		SELECT time, open, high, low, close
		FROM portfolio_metrics
		WHERE job_id = $1 AND time >= $2::timestamptz AND time <= $3::timestamptz
		ORDER BY time ASC
	`, jobID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	var points []models.PortfolioPoint
	for rows.Next() {
		var p models.PortfolioPoint
		rows.Scan(&p.Time, &p.Open, &p.High, &p.Low, &p.Close)
		points = append(points, p)
	}
	if points == nil {
		points = []models.PortfolioPoint{}
	}
	writeJSON(w, http.StatusOK, models.PortfolioResponse{Points: points})
}

// CancelJob handles POST /api/jobs/:id/cancel
func CancelJob(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := r.PathValue("id")

	var ownerID, status string
	db.Pool.QueryRow(r.Context(), `
		SELECT s.user_id, j.status FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE j.id = $1
	`, jobID).Scan(&ownerID, &status)

	if ownerID != userID {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if status != "running" {
		writeError(w, http.StatusBadRequest, "job is not running")
		return
	}

	queue.SetStopSignal(jobID)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"jobId":  jobID,
		"status": "cancelling",
	})
}
