package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// seedStrategy creates a user + strategy + version and returns userID, strategyVersionID.
func seedStrategy(t *testing.T, emailSuffix string) (userID, versionID string) {
	t.Helper()
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

	t.Run("400 on non-running job (status=queued)", func(t *testing.T) {
		req := authedReq(t, http.MethodPost, "/", "", userID, email)
		req.SetPathValue("id", jobID)
		rr := withAuth(CancelJob, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
		}
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
