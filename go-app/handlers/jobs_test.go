package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// seedStrategy creates a user + strategy + version and returns userID, strategyVersionID.
func seedStrategy(t *testing.T, emailSuffix string) (userID, versionID string) {
	t.Helper()
	srv := newFakePythonService(t, func(_ string) (string, string) { return "MyStrategy", "" })
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)
	email := "job-user-" + emailSuffix + "@test.com"
	userID = seedUser(t, email)

	req := multipartUpload(t, "test-strat-"+emailSuffix, validStrategy, userID, email)
	rr := withAuth(UploadStrategy, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("seedStrategy upload: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)
	versionID = resp["versionId"].(string)
	return userID, versionID
}

func TestSubmitJob(t *testing.T) {
	userID, versionID := seedStrategy(t, "submit")

	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})

	req := authedReq(t, http.MethodPost, "/", string(body), userID, "job-user-submit@test.com")
	rr := withAuth(SubmitJob, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	decodeJSON(t, rr, &resp)
	jobID := resp["jobId"]
	if jobID == "" {
		t.Fatal("expected jobId in response")
	}

	// Verify the job row exists in the DB with status=queued.
	var status string
	err := testPool.QueryRow(context.Background(),
		"SELECT status FROM jobs WHERE id=$1", jobID).Scan(&status)
	if err != nil {
		t.Fatalf("job not found in DB: %v", err)
	}
	if status != "queued" {
		t.Errorf("expected status=queued, got %s", status)
	}
}

func TestSubmitJob_RedisFailure(t *testing.T) {
	userID, versionID := seedStrategy(t, "redis-fail")
	email := "job-user-redis-fail@test.com"

	testRedis.SetError("forced failure")
	defer testRedis.SetError("")

	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})

	req := authedReq(t, http.MethodPost, "/", string(body), userID, email)
	rr := withAuth(SubmitJob, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}

	// Find the job row and verify it was marked failed.
	var status, errorMessage string
	err := testPool.QueryRow(context.Background(),
		"SELECT status, COALESCE(error_message, '') FROM jobs WHERE strategy_version_id=$1 ORDER BY created_at DESC LIMIT 1",
		versionID).Scan(&status, &errorMessage)
	if err != nil {
		t.Fatalf("job not found in DB: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status=failed, got %s", status)
	}
	if errorMessage == "" {
		t.Errorf("expected non-empty error_message, got empty string")
	}
}

func TestSubmitJob_NonOwnedVersion(t *testing.T) {
	_, versionID := seedStrategy(t, "nonowned-owner")
	attackerID := seedUser(t, "job-attacker@test.com")

	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})

	req := authedReq(t, http.MethodPost, "/", string(body), attackerID, "job-attacker@test.com")
	rr := withAuth(SubmitJob, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCancelJob(t *testing.T) {
	userID, versionID := seedStrategy(t, "cancel")
	email := "job-user-cancel@test.com"

	// Submit a backtest to create a job.
	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})
	submitReq := authedReq(t, http.MethodPost, "/", string(body), userID, email)
	submitRR := withAuth(SubmitJob, submitReq)
	if submitRR.Code != http.StatusAccepted {
		t.Fatalf("submit: expected 202, got %d", submitRR.Code)
	}
	var submitResp map[string]string
	decodeJSON(t, submitRR, &submitResp)
	jobID := submitResp["jobId"]

	t.Run("202 on queued job (direct DB cancel)", func(t *testing.T) {
		req := authedReq(t, http.MethodPost, "/", "", userID, email)
		req.SetPathValue("id", jobID)
		rr := withAuth(CancelJob, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
		}
		var status string
		if err := testPool.QueryRow(context.Background(),
			"SELECT status FROM jobs WHERE id=$1", jobID).Scan(&status); err != nil {
			t.Fatalf("query job status: %v", err)
		}
		if status != "failed" {
			t.Errorf("expected status=failed after queued cancel, got %s", status)
		}
		// Restore status to queued for subsequent sub-tests.
		_, _ = testPool.Exec(context.Background(),
			"UPDATE jobs SET status='queued', completed_at=NULL, error_message=NULL WHERE id=$1", jobID)
	})

	t.Run("202 on running job", func(t *testing.T) {
		// Promote job to running status directly in the DB.
		_, err := testPool.Exec(context.Background(),
			"UPDATE jobs SET status='running' WHERE id=$1", jobID)
		if err != nil {
			t.Fatalf("update job status: %v", err)
		}

		req := authedReq(t, http.MethodPost, "/", "", userID, email)
		req.SetPathValue("id", jobID)
		rr := withAuth(CancelJob, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]string
		decodeJSON(t, rr, &resp)
		if resp["status"] != "cancelling" {
			t.Errorf("expected status=cancelling, got %s", resp["status"])
		}
	})
}

func TestGetJobMetrics_NotReady(t *testing.T) {
	userID, versionID := seedStrategy(t, "metrics")
	email := "job-user-metrics@test.com"

	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})
	submitReq := authedReq(t, http.MethodPost, "/", string(body), userID, email)
	submitRR := withAuth(SubmitJob, submitReq)
	if submitRR.Code != http.StatusAccepted {
		t.Fatalf("submit: expected 202, got %d", submitRR.Code)
	}
	var submitResp map[string]string
	decodeJSON(t, submitRR, &submitResp)
	jobID := submitResp["jobId"]

	// Metrics not yet available — expect 404.
	req := authedReq(t, http.MethodGet, "/", "", userID, email)
	req.SetPathValue("id", jobID)
	rr := withAuth(GetJobMetrics, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCancelJob_QueuedJob(t *testing.T) {
	userID, versionID := seedStrategy(t, "cancel-queued")
	email := "job-user-cancel-queued@test.com"

	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})
	submitReq := authedReq(t, http.MethodPost, "/", string(body), userID, email)
	submitRR := withAuth(SubmitJob, submitReq)
	if submitRR.Code != http.StatusAccepted {
		t.Fatalf("submit: expected 202, got %d", submitRR.Code)
	}
	var submitResp map[string]string
	decodeJSON(t, submitRR, &submitResp)
	jobID := submitResp["jobId"]

	// Job is queued — cancel should succeed with 202.
	req := authedReq(t, http.MethodPost, "/", "", userID, email)
	req.SetPathValue("id", jobID)
	rr := withAuth(CancelJob, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify the job was directly marked failed in the DB.
	var status string
	if err := testPool.QueryRow(context.Background(),
		"SELECT status FROM jobs WHERE id=$1", jobID).Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status=failed after queued cancel, got %s", status)
	}
}

func TestCancelJob_RedisError(t *testing.T) {
	userID, versionID := seedStrategy(t, "cancel-redis-err")
	email := "job-user-cancel-redis-err@test.com"

	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})
	submitReq := authedReq(t, http.MethodPost, "/", string(body), userID, email)
	submitRR := withAuth(SubmitJob, submitReq)
	if submitRR.Code != http.StatusAccepted {
		t.Fatalf("submit: expected 202, got %d", submitRR.Code)
	}
	var submitResp map[string]string
	decodeJSON(t, submitRR, &submitResp)
	jobID := submitResp["jobId"]

	// Promote job to running.
	_, err := testPool.Exec(context.Background(),
		"UPDATE jobs SET status='running' WHERE id=$1", jobID)
	if err != nil {
		t.Fatalf("update job status: %v", err)
	}

	// Force Redis error so SetStopSignal fails.
	testRedis.SetError("forced failure")
	defer testRedis.SetError("")

	req := authedReq(t, http.MethodPost, "/", "", userID, email)
	req.SetPathValue("id", jobID)
	rr := withAuth(CancelJob, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to cancel job") {
		t.Errorf("expected 'failed to cancel job' in response, got: %s", rr.Body.String())
	}
}

func TestCancelJob_QueuedResponse(t *testing.T) {
	userID, versionID := seedStrategy(t, "cancel-queued-resp")
	email := "job-user-cancel-queued-resp@test.com"

	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})
	submitReq := authedReq(t, http.MethodPost, "/", string(body), userID, email)
	submitRR := withAuth(SubmitJob, submitReq)
	if submitRR.Code != http.StatusAccepted {
		t.Fatalf("submit: expected 202, got %d", submitRR.Code)
	}
	var submitResp map[string]string
	decodeJSON(t, submitRR, &submitResp)
	jobID := submitResp["jobId"]

	// Cancel the queued job.
	req := authedReq(t, http.MethodPost, "/", "", userID, email)
	req.SetPathValue("id", jobID)
	rr := withAuth(CancelJob, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	// Response body must contain status="failed" and message="cancelled by user".
	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["status"] != "failed" {
		t.Errorf("expected status=failed in response body, got %q", resp["status"])
	}
	if resp["message"] != "cancelled by user" {
		t.Errorf("expected message='cancelled by user' in response body, got %q", resp["message"])
	}

	// DB row must have status=failed and error_message='cancelled by user'.
	var dbStatus, dbErrMsg string
	if err := testPool.QueryRow(context.Background(),
		"SELECT status, COALESCE(error_message, '') FROM jobs WHERE id=$1", jobID).
		Scan(&dbStatus, &dbErrMsg); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if dbStatus != "failed" {
		t.Errorf("expected DB status=failed, got %s", dbStatus)
	}
	if dbErrMsg != "cancelled by user" {
		t.Errorf("expected DB error_message='cancelled by user', got %q", dbErrMsg)
	}
}

func TestGetJob_DBError(t *testing.T) {
	userID := seedUser(t, "getjob-dberr@test.com")

	// Pass an invalid UUID — Postgres will return a parse error (not ErrNoRows).
	req := authedReq(t, http.MethodGet, "/", "", userID, "getjob-dberr@test.com")
	req.SetPathValue("id", "not-a-uuid")
	rr := withAuth(GetJob, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "database error") {
		t.Errorf("expected 'database error' in response, got: %s", rr.Body.String())
	}
}

func TestListJobsLimitCap(t *testing.T) {
	userID := seedUser(t, "limit-cap@test.com")
	req := authedReq(t, http.MethodGet, "/?limit=201", "", userID, "limit-cap@test.com")
	rr := withAuth(ListJobs, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "limit must be between 1 and 200") {
		t.Errorf("expected limit error message, got: %s", rr.Body.String())
	}
}
