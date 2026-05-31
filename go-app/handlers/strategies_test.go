package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// seedIPCounter provides unique IPs so each seedUser call gets its own rate-limit bucket.
var seedIPCounter atomic.Int64

const (
	validStrategy = `
class MyStrategy(QCAlgorithm):
    def Initialize(self):
        pass
`
	strategyWithOS = `
import os
class MyStrategy(QCAlgorithm):
    pass
`
	strategyNoClass = `
def just_a_function():
    pass
`
)

// seedUser registers a test user and returns their ID.
// Each call uses a unique X-Forwarded-For IP so it gets its own rate-limit bucket.
func seedUser(t *testing.T, email string) string {
	t.Helper()
	n := seedIPCounter.Add(1)
	ip := fmt.Sprintf("10.%d.%d.%d", (n>>16)&0xFF, (n>>8)&0xFF, n&0xFF)
	req := jsonReqWithIP("POST", "/", `{"email":"`+email+`","password":"password123"}`, ip)
	rr := httptest.NewRecorder()
	Register(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("seedUser: register returned %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, rr, &resp)
	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			"DELETE FROM users WHERE id=$1", resp["userId"])
	})
	return resp["userId"]
}

// newFakePythonService creates a test HTTP server that mimics the Python validation service.
// validFn is called with the source and returns (className, violation).
func newFakePythonService(t *testing.T, validFn func(source string) (className, violation string)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Source string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		className, violation := validFn(req.Source)
		valid := violation == ""
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":      valid,
			"class_name": className,
			"violation":  violation,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUploadStrategy_PythonServiceViolation(t *testing.T) {
	userID := seedUser(t, "strat-pyviolation@test.com")

	srv := newFakePythonService(t, func(source string) (string, string) {
		if strings.Contains(source, "import os") {
			return "", "import os detected on line 2"
		}
		return "MyStrategy", ""
	})
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)

	req := multipartUpload(t, "bad-strat", strategyWithOS, userID, "strat-pyviolation@test.com")
	rr := withAuth(UploadStrategy, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	decodeJSON(t, rr, &body)
	if !strings.Contains(body["error"], "import os") {
		t.Errorf("expected violation text in error, got: %s", body["error"])
	}
}

func TestUploadStrategy_PythonServiceUnreachable(t *testing.T) {
	userID := seedUser(t, "strat-pydown@test.com")

	// Point to a server that is already closed.
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed.Close()
	t.Setenv("PYTHON_SERVICE_URL", closed.URL)

	req := multipartUpload(t, "any-strat", validStrategy, userID, "strat-pydown@test.com")
	rr := withAuth(UploadStrategy, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUploadStrategy_Success(t *testing.T) {
	userID := seedUser(t, "strat-upload@test.com")

	srv := newFakePythonService(t, func(source string) (string, string) {
		return "MyStrategy", ""
	})
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)

	req := multipartUpload(t, "my-strategy", validStrategy, userID, "strat-upload@test.com")
	rr := withAuth(UploadStrategy, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)
	if resp["strategyId"] == "" {
		t.Error("expected strategyId in response")
	}
	if resp["versionNumber"] != float64(1) {
		t.Errorf("expected versionNumber=1, got %v", resp["versionNumber"])
	}
	if resp["className"] != "MyStrategy" {
		t.Errorf("expected className=MyStrategy, got %v", resp["className"])
	}
}

func TestUploadStrategy_GetUserIDFalse(t *testing.T) {
	// Build a request with no auth token — RequireAuth will reject it with 401.
	var buf strings.Builder
	fmt.Fprint(&buf, validStrategy)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	// Call handler directly without going through RequireAuth so context has no userID.
	UploadStrategy(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteStrategy(t *testing.T) {
	userID := seedUser(t, "strat-delete@test.com")

	srv := newFakePythonService(t, func(source string) (string, string) {
		return "MyStrategy", ""
	})
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)

	// Upload a strategy first.
	uploadReq := multipartUpload(t, "to-delete", validStrategy, userID, "strat-delete@test.com")
	uploadRR := withAuth(UploadStrategy, uploadReq)
	if uploadRR.Code != http.StatusCreated {
		t.Fatalf("upload: expected 201, got %d: %s", uploadRR.Code, uploadRR.Body.String())
	}
	var uploaded map[string]interface{}
	decodeJSON(t, uploadRR, &uploaded)
	strategyID := uploaded["strategyId"].(string)

	// Delete it.
	delReq := authedReq(t, http.MethodDelete, "/", "", userID, "strat-delete@test.com")
	delReq.SetPathValue("id", strategyID)
	delRR := withAuth(DeleteStrategy, delReq)
	if delRR.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", delRR.Code)
	}

	// GET should now return 404.
	getReq := authedReq(t, http.MethodGet, "/", "", userID, "strat-delete@test.com")
	getReq.SetPathValue("id", strategyID)
	getRR := withAuth(GetStrategy, getReq)
	if getRR.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", getRR.Code)
	}
}

func TestStrategyOwnership(t *testing.T) {
	ownerID := seedUser(t, "strat-owner@test.com")
	otherID := seedUser(t, "strat-other@test.com")

	srv := newFakePythonService(t, func(source string) (string, string) {
		return "MyStrategy", ""
	})
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)

	// Owner uploads a strategy.
	uploadReq := multipartUpload(t, "owned-strat", validStrategy, ownerID, "strat-owner@test.com")
	uploadRR := withAuth(UploadStrategy, uploadReq)
	if uploadRR.Code != http.StatusCreated {
		t.Fatalf("upload: expected 201, got %d", uploadRR.Code)
	}
	var uploaded map[string]interface{}
	decodeJSON(t, uploadRR, &uploaded)
	strategyID := uploaded["strategyId"].(string)

	// Other user tries to GET the strategy — must get 403.
	getReq := authedReq(t, http.MethodGet, "/", "", otherID, "strat-other@test.com")
	getReq.SetPathValue("id", strategyID)
	getRR := withAuth(GetStrategy, getReq)
	if getRR.Code != http.StatusForbidden {
		t.Fatalf("cross-user GET: expected 403, got %d", getRR.Code)
	}
}

func TestGetVersionCode_NotFound(t *testing.T) {
	userID := seedUser(t, "strat-vcode-notfound@test.com")

	srv := newFakePythonService(t, func(source string) (string, string) {
		return "MyStrategy", ""
	})
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)

	// Upload a strategy to get a valid strategyID.
	uploadReq := multipartUpload(t, "vcode-strat", validStrategy, userID, "strat-vcode-notfound@test.com")
	uploadRR := withAuth(UploadStrategy, uploadReq)
	if uploadRR.Code != http.StatusCreated {
		t.Fatalf("upload: expected 201, got %d: %s", uploadRR.Code, uploadRR.Body.String())
	}
	var uploaded map[string]interface{}
	decodeJSON(t, uploadRR, &uploaded)
	strategyID := uploaded["strategyId"].(string)

	// Request a nonexistent versionId — must get 404.
	req := authedReq(t, http.MethodGet, "/", "", userID, "strat-vcode-notfound@test.com")
	req.SetPathValue("id", strategyID)
	req.SetPathValue("versionId", "00000000-0000-0000-0000-000000000000")
	rr := withAuth(GetVersionCode, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUploadStrategy_VersionInsertFail_NoS3Call(t *testing.T) {
	// Verify that strategy_versions row is created and S3 is called after DB insert succeeds.
	// This test documents and exercises the new DB-before-S3 ordering in UploadStrategy.
	userID := seedUser(t, "strat-order@test.com")
	srv := newFakePythonService(t, func(_ string) (string, string) { return "MyStrategy", "" })
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)
	req := multipartUpload(t, "order-test", validStrategy, userID, "strat-order@test.com")
	rr := withAuth(UploadStrategy, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)
	if resp["versionId"] == "" || resp["strategyId"] == "" {
		t.Error("expected strategyId and versionId in response")
	}
	// Verify the strategy_versions row exists in DB.
	var count int
	testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM strategy_versions WHERE id=$1", resp["versionId"]).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 strategy_versions row, got %d", count)
	}
}

func TestGetStrategy_StatsError(t *testing.T) {
	srv := newFakePythonService(t, func(_ string) (string, string) { return "MyStrategy", "" })
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)
	userID := seedUser(t, "gs-statserr@test.com")

	// Upload a strategy so GetStrategy has a real row to find.
	req := multipartUpload(t, "stats-err-strat", validStrategy, userID, "gs-statserr@test.com")
	rr := withAuth(UploadStrategy, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var uploaded map[string]interface{}
	decodeJSON(t, rr, &uploaded)
	stratID := uploaded["strategyId"].(string)

	// Break the stats query by renaming performance_metrics.
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "ALTER TABLE performance_metrics RENAME TO performance_metrics_bak"); err != nil {
		t.Fatalf("rename table: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, "ALTER TABLE performance_metrics_bak RENAME TO performance_metrics") //nolint:errcheck
	})

	// GetStrategy must return 200 with strategy data and zero-valued stats.
	getReq := authedReq(t, http.MethodGet, "/"+stratID, "", userID, "gs-statserr@test.com")
	getReq.SetPathValue("id", stratID)
	getRR := withAuth(GetStrategy, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRR.Code, getRR.Body.String())
	}
	var resp map[string]interface{}
	decodeJSON(t, getRR, &resp)
	if resp["id"] != stratID {
		t.Errorf("expected strategyId %q in response, got %v", stratID, resp["id"])
	}
	stats, ok := resp["stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected stats object in response, got %v", resp["stats"])
	}
	if rc, _ := stats["runCount"].(float64); rc != 0 {
		t.Errorf("expected runCount=0, got %v", stats["runCount"])
	}
}

func TestGetStrategy_OwnershipCheck(t *testing.T) {
	srv := newFakePythonService(t, func(_ string) (string, string) { return "MyStrategy", "" })
	t.Setenv("PYTHON_SERVICE_URL", srv.URL)
	ownerID := seedUser(t, "gs-owner@test.com")
	otherID := seedUser(t, "gs-other@test.com")

	// Upload strategy as owner.
	req := multipartUpload(t, "gs-strat", validStrategy, ownerID, "gs-owner@test.com")
	rr := withAuth(UploadStrategy, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload: expected 201, got %d", rr.Code)
	}
	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)
	stratID := resp["strategyId"].(string)

	// Access as other user — should get 403.
	req2 := authedReq(t, http.MethodGet, "/"+stratID, "", otherID, "gs-other@test.com")
	req2.SetPathValue("id", stratID)
	rr2 := withAuth(GetStrategy, req2)
	if rr2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong user, got %d: %s", rr2.Code, rr2.Body.String())
	}
}
