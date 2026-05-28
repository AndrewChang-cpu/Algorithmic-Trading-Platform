package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
func seedUser(t *testing.T, email string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	Register(rr, jsonReq("POST", "/", `{"email":"`+email+`","password":"password123"}`))
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
