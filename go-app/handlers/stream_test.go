package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"application-server/middleware"

	"github.com/gorilla/websocket"
)

func TestHealthCheck_OK(t *testing.T) {
	// testRedis is initialized in testhelper_test.go (TestMain)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	HealthCheck(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"status":"ok"`) && !strings.Contains(body, `"status": "ok"`) {
		t.Errorf("expected status:ok in body, got: %s", body)
	}
	if strings.Contains(body, `"kafka"`) || strings.Contains(body, `"redis"`) || strings.Contains(body, `"db"`) {
		t.Errorf("health body should not expose internal fields: %s", body)
	}
}

// TestJobStatusStream_WriteError verifies that JobStatusStream closes rows and
// returns cleanly when a WebSocket write fails (i.e., client disconnects mid-stream).
func TestJobStatusStream_WriteError(t *testing.T) {
	// Allow the upgrader to accept connections from the test server's origin.
	origOrigins := allowedOrigins
	allowedOrigins = []string{"http://127.0.0.1"}
	defer func() { allowedOrigins = origOrigins }()

	// Seed a user and a job with a log row so the handler will attempt a write.
	userID := seedUser(t, "stream-write-error@test.com")
	email := "stream-write-error@test.com"

	userID2, versionID := seedStrategy(t, "stream-write-err")
	_ = userID2
	// We need a job owned by userID, so we create one directly.
	// Re-seed with a strategy owned by userID.
	srv := newFakePythonService(t, func(_ string) (string, string) { return "MyStrategy", "" })
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)

	uploadReq := multipartUpload(t, "stream-err-strat", validStrategy, userID, email)
	uploadRR := withAuth(UploadStrategy, uploadReq)
	if uploadRR.Code != http.StatusCreated {
		t.Fatalf("upload: expected 201, got %d: %s", uploadRR.Code, uploadRR.Body.String())
	}
	var uploadResp map[string]interface{}
	decodeJSON(t, uploadRR, &uploadResp)
	_ = versionID
	ownedVersionID := uploadResp["versionId"].(string)

	submitBody, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": ownedVersionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})
	submitReq := authedReq(t, http.MethodPost, "/", string(submitBody), userID, email)
	submitRR := withAuth(SubmitJob, submitReq)
	if submitRR.Code != http.StatusAccepted {
		t.Fatalf("submit: expected 202, got %d: %s", submitRR.Code, submitRR.Body.String())
	}
	var submitResp map[string]string
	decodeJSON(t, submitRR, &submitResp)
	jobID := submitResp["jobId"]

	// Insert a log row so the handler will try to write at least one message.
	_, err := testPool.Exec(context.Background(),
		"INSERT INTO job_logs (job_id, level, message) VALUES ($1, 'info', 'hello')",
		jobID)
	if err != nil {
		t.Fatalf("insert job_log: %v", err)
	}

	// Start a real HTTP server for the WS upgrade.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", jobID)
		JobStatusStream(w, r)
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/stream/jobs/" + jobID
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	header := http.Header{}
	header.Set("Origin", "http://127.0.0.1")

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Send auth message.
	tok, err := middleware.GenerateAccessToken(userID, email)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	authMsg, _ := json.Marshal(map[string]string{"type": "auth", "token": tok})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	// Read auth_ok.
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read auth_ok: %v", err)
	}
	if !strings.Contains(string(msg), "auth_ok") {
		t.Fatalf("expected auth_ok, got: %s", msg)
	}

	// Close the connection abruptly to cause the next WriteMessage in the handler to fail.
	conn.Close()

	// The handler should exit cleanly within a short window.
	// We verify this by checking the test server shuts down without hanging — the
	// handler goroutine must release within the deadline below.
	done := make(chan struct{})
	go func() {
		ts.Close()
		close(done)
	}()

	select {
	case <-done:
		// handler exited cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("JobStatusStream did not exit after client disconnect (possible resource leak)")
	}
}
