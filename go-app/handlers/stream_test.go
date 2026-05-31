package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	// We verify this by checking the test server shuts down without hanging -- the
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

// dialPortfolioStream dials the PortfolioStream handler on ts, sends the auth message,
// and returns the open connection. The caller is responsible for reading and closing it.
func dialPortfolioStream(t *testing.T, ts *httptest.Server, jobID, userID, email string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/stream/portfolio/" + jobID
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	header := http.Header{}
	header.Set("Origin", "http://127.0.0.1")

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	tok, err := middleware.GenerateAccessToken(userID, email)
	if err != nil {
		conn.Close()
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	authMsg, _ := json.Marshal(map[string]string{"type": "auth", "token": tok})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		conn.Close()
		t.Fatalf("write auth: %v", err)
	}
	return conn
}

// readCloseFrame reads messages from conn until it receives a close frame.
// Returns (closeCode, closeText), or (-1, errMsg) for non-close errors.
func readCloseFrame(conn *websocket.Conn) (int, string) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if ce, ok := err.(*websocket.CloseError); ok {
				return ce.Code, ce.Text
			}
			return -1, err.Error()
		}
	}
}

// TestPortfolioStream_DBError verifies that a DB error on the ownership query closes
// the WebSocket with code 1011 (InternalServerErr) and message "server error", not
// 1008 "forbidden" (which would mask the real error as an authorization failure).
func TestPortfolioStream_DBError(t *testing.T) {
	origOrigins := allowedOrigins
	allowedOrigins = []string{"http://127.0.0.1"}
	defer func() { allowedOrigins = origOrigins }()

	userID := seedUser(t, "portfolio-db-err@test.com")
	email := "portfolio-db-err@test.com"

	// Use a non-UUID jobId so Postgres returns a parse error instead of ErrNoRows.
	const badJobID = "not-a-valid-uuid"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("jobId", badJobID)
		PortfolioStream(w, r)
	}))
	defer ts.Close()

	conn := dialPortfolioStream(t, ts, badJobID, userID, email)
	defer conn.Close()

	code, text := readCloseFrame(conn)
	if code != websocket.CloseInternalServerErr {
		t.Errorf("expected close code %d (InternalServerErr), got %d (text: %q)",
			websocket.CloseInternalServerErr, code, text)
	}
	if text != "server error" {
		t.Errorf("expected close text %q, got %q", "server error", text)
	}
}

// semTestCounter provides unique email suffixes for semaphore tests.
var semTestCounter int64

// TestPortfolioStream_SemaphoreNotConsumedOnAuthFail verifies that ownership-mismatch
// rejections do not consume semaphore slots, allowing a legitimate follow-up connection
// from the same user to succeed.
//
// Strategy: launch maxWSPerUser concurrent goroutines that each dial the handler with a
// foreign job ID (ownership mismatch). Each goroutine reads auth_ok then waits for a
// barrier before it drains the close frame -- this holds the handler goroutines in the
// semaphore-held window (for the buggy code) while the 6th connection is attempted.
// After the fix, auth failures never acquire a semaphore slot, so the 6th request for
// the user's own job must not be rejected with "too many connections".
func TestPortfolioStream_SemaphoreNotConsumedOnAuthFail(t *testing.T) {
	origOrigins := allowedOrigins
	allowedOrigins = []string{"http://127.0.0.1"}
	defer func() { allowedOrigins = origOrigins }()

	// Create job owned by ownerID.
	ownerID, versionID := seedStrategy(t, "semaphore-owner")
	ownerEmail := "job-user-semaphore-owner@test.com"

	body, _ := json.Marshal(map[string]interface{}{
		"strategyVersionId": versionID,
		"type":              "backtest",
		"symbols":           []string{"SPY"},
		"resolution":        "1d",
		"startDate":         "2024-01-01",
		"endDate":           "2024-03-31",
	})
	submitReq := authedReq(t, http.MethodPost, "/", string(body), ownerID, ownerEmail)
	submitRR := withAuth(SubmitJob, submitReq)
	if submitRR.Code != http.StatusAccepted {
		t.Fatalf("submit: expected 202, got %d: %s", submitRR.Code, submitRR.Body.String())
	}
	var submitResp map[string]string
	decodeJSON(t, submitRR, &submitResp)
	ownedJobID := submitResp["jobId"]

	// Create an attacker user (valid JWT, does not own the job).
	n := atomic.AddInt64(&semTestCounter, 1)
	attackerEmail := fmt.Sprintf("semaphore-attacker-%d@test.com", n)
	attackerID := seedUser(t, attackerEmail)

	// Ensure a clean semaphore state.
	wsSemaphore.Delete(attackerID)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		PortfolioStream(w, r)
	}))
	defer ts.Close()

	// ready: each goroutine signals here after receiving auth_ok (server sent it,
	//        meaning the handler has passed wsFirstMessageAuth).
	// release: closed to let goroutines drain their connections.
	ready := make(chan struct{}, maxWSPerUser)
	release := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < maxWSPerUser; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := dialPortfolioStream(t, ts, ownedJobID, attackerID, attackerEmail)
			defer conn.Close()

			// Read the first message from the server (auth_ok or close frame).
			conn.SetReadDeadline(time.Now().Add(3 * time.Second)) //nolint:errcheck
			_, firstMsg, err := conn.ReadMessage()
			if err == nil && strings.Contains(string(firstMsg), "auth_ok") {
				// Signal that we are past auth. The buggy handler has already
				// incremented the semaphore at this point; the fixed handler has not.
				ready <- struct{}{}
				// Wait for the test to send the 6th connection before we drain.
				<-release
			}
			// Drain any remaining messages / close frame.
			readCloseFrame(conn) //nolint:errcheck
		}()
	}

	// Wait until all maxWSPerUser goroutines have sent auth_ok signal (or timed out).
	timeout := time.After(10 * time.Second)
	for i := 0; i < maxWSPerUser; i++ {
		select {
		case <-ready:
		case <-timeout:
			close(release) // unblock goroutines before failing
			wg.Wait()
			t.Fatalf("timed out waiting for goroutine %d to reach auth_ok", i+1)
		}
	}

	// At this point all maxWSPerUser handler goroutines have passed wsFirstMessageAuth.
	// With the bug (semaphore before ownership check): counter == maxWSPerUser.
	// With the fix (semaphore after ownership check): counter == 0 (ownership check
	// has either not been reached yet or has already failed without touching semaphore).
	//
	// Make the 6th request and check whether it is rejected for "too many connections".
	conn6 := dialPortfolioStream(t, ts, ownedJobID, attackerID, attackerEmail)
	code6, text6 := readCloseFrame(conn6)
	conn6.Close()

	// Unblock the waiting goroutines and wait for them to finish.
	close(release)
	wg.Wait()

	if text6 == "too many connections" {
		t.Errorf("6th request rejected with %q (code %d): semaphore incorrectly consumed by auth failures",
			text6, code6)
	}
}
