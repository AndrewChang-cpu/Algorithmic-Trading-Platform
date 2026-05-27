package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestUploadStrategy_Violations(t *testing.T) {
	userID := seedUser(t, "strat-violations@test.com")

	t.Run("422 on blocked import", func(t *testing.T) {
		req := multipartUpload(t, "bad-strat", strategyWithOS, userID, "strat-violations@test.com")
		rr := withAuth(UploadStrategy, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("422 on missing QCAlgorithm subclass", func(t *testing.T) {
		req := multipartUpload(t, "bad-strat2", strategyNoClass, userID, "strat-violations@test.com")
		rr := withAuth(UploadStrategy, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}

func TestUploadStrategy_Success(t *testing.T) {
	userID := seedUser(t, "strat-upload@test.com")
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
}

func TestDeleteStrategy(t *testing.T) {
	userID := seedUser(t, "strat-delete@test.com")

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
