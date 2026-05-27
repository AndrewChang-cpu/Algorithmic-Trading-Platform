package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"application-server/db"
	"application-server/middleware"
	"application-server/queue"
	s3client "application-server/s3"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	testPool  *pgxpool.Pool
	testRedis *miniredis.Miniredis
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	// --- Postgres container ---
	pgc, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:16-alpine",
			Env: map[string]string{
				"POSTGRES_PASSWORD": "testpass",
				"POSTGRES_USER":     "testuser",
				"POSTGRES_DB":       "testdb",
			},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForListeningPort("5432/tcp"),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres container: %v\n", err)
		os.Exit(1)
	}
	defer pgc.Terminate(ctx) //nolint:errcheck

	host, _ := pgc.Host(ctx)
	port, _ := pgc.MappedPort(ctx, "5432")
	dsn := fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb?sslmode=disable", host, port.Port())

	if err := db.Init(dsn); err != nil {
		fmt.Fprintf(os.Stderr, "db.Init: %v\n", err)
		os.Exit(1)
	}
	testPool = db.Pool

	if err := applyTestMigrations(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrations: %v\n", err)
		os.Exit(1)
	}

	// --- Fake S3 server ---
	fakeS3 := newFakeS3Server()
	defer fakeS3.Close()
	if err := s3client.Init(fakeS3.URL, "key", "secret", "test-bucket", "us-east-1"); err != nil {
		fmt.Fprintf(os.Stderr, "s3.Init: %v\n", err)
		os.Exit(1)
	}

	// --- miniredis ---
	testRedis, err = miniredis.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "miniredis: %v\n", err)
		os.Exit(1)
	}
	defer testRedis.Close()
	if err := queue.Init("redis://" + testRedis.Addr()); err != nil {
		fmt.Fprintf(os.Stderr, "queue.Init: %v\n", err)
		os.Exit(1)
	}

	// --- JWT keys ---
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	middleware.SetKeysForTest(priv, &priv.PublicKey)

	os.Exit(m.Run())
}

// applyTestMigrations runs migrations 001-006 and 009.
// Migrations 007 and 008 call create_hypertable() which requires TimescaleDB.
func applyTestMigrations(ctx context.Context) error {
	// Go tests set the working directory to the package directory.
	// From go-app/handlers/, ../../migrations is the project-root/migrations/.
	migrationsDir := filepath.Join("..", "..", "migrations")

	for _, f := range []string{
		"001_create_users.up.sql",
		"002_create_strategies.up.sql",
		"003_create_strategy_versions.up.sql",
		"004_create_jobs.up.sql",
		"005_create_job_logs.up.sql",
		"006_create_performance_metrics.up.sql",
		"009_create_refresh_tokens.up.sql",
	} {
		data, err := os.ReadFile(filepath.Join(migrationsDir, f))
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if _, err := testPool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("exec %s: %w", f, err)
		}
	}
	return nil
}

// newFakeS3Server returns a minimal S3-compatible httptest server.
// It accepts PutObject, GetObject, ListObjectsV2, and DeleteObject requests.
func newFakeS3Server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			io.Copy(io.Discard, r.Body) //nolint:errcheck
			w.Header().Set("ETag", `"test-etag"`)
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			if r.URL.Query().Get("list-type") != "" {
				w.Header().Set("Content-Type", "application/xml")
				fmt.Fprint(w,
					`<?xml version="1.0" encoding="UTF-8"?>`+
						`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`+
						`<Name>test-bucket</Name><IsTruncated>false</IsTruncated>`+
						`<KeyCount>0</KeyCount></ListBucketResult>`)
			} else {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "class TestStrategy(QCAlgorithm):\n    pass\n")
			}

		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

// --- Test helpers ---

// jsonReq builds a POST/GET/DELETE request with a JSON body.
func jsonReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// authedReq builds a request that carries a valid JWT for the given userID+email.
// The caller should still pass it through RequireAuth (or call withAuth) to populate the context.
func authedReq(t *testing.T, method, target, body, userID, email string) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	tok, err := middleware.GenerateAccessToken(userID, email)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

// withAuth wraps a handler in RequireAuth and serves the request, returning the recorder.
func withAuth(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	middleware.RequireAuth(h).ServeHTTP(rr, req)
	return rr
}

// multipartUpload builds a multipart/form-data request suitable for UploadStrategy.
func multipartUpload(t *testing.T, name, code, userID, email string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", name)
	fw, err := mw.CreateFormFile("file", "strategy.py")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	fmt.Fprint(fw, code)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	tok, _ := middleware.GenerateAccessToken(userID, email)
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

// decodeJSON is a test helper that decodes the recorder body into dst.
func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, dst interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(dst); err != nil {
		t.Fatalf("decodeJSON: %v (body: %s)", err, rr.Body.String())
	}
}
