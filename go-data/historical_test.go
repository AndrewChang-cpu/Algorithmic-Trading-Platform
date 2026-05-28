package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const createMarketDataSQL = `
CREATE TABLE IF NOT EXISTS market_data (
  time        TIMESTAMPTZ NOT NULL,
  symbol      VARCHAR(20) NOT NULL,
  resolution  VARCHAR(10) NOT NULL,
  open        DECIMAL(20,4),
  high        DECIMAL(20,4),
  low         DECIMAL(20,4),
  close       DECIMAL(20,4),
  volume      BIGINT
);
CREATE UNIQUE INDEX IF NOT EXISTS market_data_symbol_resolution_time_idx
  ON market_data (symbol, resolution, time DESC);
`

func TestMain(m *testing.M) {
	ctx := context.Background()

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

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgxpool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, createMarketDataSQL); err != nil {
		fmt.Fprintf(os.Stderr, "create table: %v\n", err)
		os.Exit(1)
	}

	db = pool
	os.Setenv("ALPACA_API_KEY", "test-key")
	os.Setenv("ALPACA_API_SECRET", "test-secret")

	os.Exit(m.Run())
}

// mockAlpacaServer returns an httptest server that serves 4 SPY daily bars.
// requestCount is incremented on each request.
func mockAlpacaServer(t *testing.T, requestCount *atomic.Int32) *httptest.Server {
	t.Helper()
	bars := []map[string]interface{}{
		{"t": "2024-01-02T00:00:00Z", "o": 476.0, "h": 477.5, "l": 475.0, "c": 476.5, "v": 10000},
		{"t": "2024-01-03T00:00:00Z", "o": 476.5, "h": 478.0, "l": 475.5, "c": 477.0, "v": 11000},
		{"t": "2024-01-04T00:00:00Z", "o": 477.0, "h": 479.0, "l": 476.0, "c": 478.5, "v": 12000},
		{"t": "2024-01-05T00:00:00Z", "o": 478.5, "h": 480.0, "l": 477.5, "c": 479.0, "v": 13000},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"bars": bars})
	}))
}

func postHistorical(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/data/historical", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleHistorical(rr, req)
	return rr
}

func TestFetchAndInsert(t *testing.T) {
	// Clean slate
	db.Exec(context.Background(), "DELETE FROM market_data WHERE symbol='SPY'")

	var count atomic.Int32
	srv := mockAlpacaServer(t, &count)
	defer srv.Close()
	alpacaBaseURL = srv.URL

	body := `{"symbols":["SPY"],"start_date":"2024-01-02","end_date":"2024-01-05","resolution":"1d"}`
	rr := postHistorical(t, body)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]int
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["bars_ready"] != 4 {
		t.Errorf("expected bars_ready=4, got %d", resp["bars_ready"])
	}

	var dbCount int
	db.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM market_data WHERE symbol='SPY'",
	).Scan(&dbCount)
	if dbCount != 4 {
		t.Errorf("expected 4 rows in DB, got %d", dbCount)
	}
}

func TestDeduplication(t *testing.T) {
	// Clean slate (first call inserts 4 bars)
	db.Exec(context.Background(), "DELETE FROM market_data WHERE symbol='SPY'")

	var count atomic.Int32
	srv := mockAlpacaServer(t, &count)
	defer srv.Close()
	alpacaBaseURL = srv.URL

	body := `{"symbols":["SPY"],"start_date":"2024-01-02","end_date":"2024-01-05","resolution":"1d"}`

	// First call — should hit Alpaca and insert 4 bars.
	rr1 := postHistorical(t, body)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first call: expected 200, got %d: %s", rr1.Code, rr1.Body.String())
	}
	if count.Load() != 1 {
		t.Errorf("expected Alpaca called once after first request, got %d", count.Load())
	}

	// Second identical call — full range covered, should NOT hit Alpaca.
	rr2 := postHistorical(t, body)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]int
	json.NewDecoder(rr2.Body).Decode(&resp2)
	if resp2["bars_ready"] != 4 {
		t.Errorf("second call: expected bars_ready=4, got %d", resp2["bars_ready"])
	}
	if count.Load() != 1 {
		t.Errorf("expected Alpaca called exactly once after second request, got %d calls", count.Load())
	}

	// Delete 2 rows to create a gap — gap detection should trigger a third Alpaca call.
	db.Exec(context.Background(),
		"DELETE FROM market_data WHERE symbol='SPY' AND time IN (SELECT time FROM market_data WHERE symbol='SPY' ORDER BY time LIMIT 2)",
	)

	var dbCountAfterDelete int
	db.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM market_data WHERE symbol='SPY'",
	).Scan(&dbCountAfterDelete)
	if dbCountAfterDelete != 2 {
		t.Fatalf("expected 2 rows after delete, got %d", dbCountAfterDelete)
	}

	// Third call — gap detected, Alpaca must be called again.
	rr3 := postHistorical(t, body)
	if rr3.Code != http.StatusOK {
		t.Fatalf("third call: expected 200, got %d: %s", rr3.Code, rr3.Body.String())
	}
	if count.Load() != 2 {
		t.Errorf("expected Alpaca called twice total (gap detected), got %d calls", count.Load())
	}

	// ON CONFLICT DO NOTHING restores deleted rows — should be back to 4.
	var dbCountAfterRefetch int
	db.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM market_data WHERE symbol='SPY'",
	).Scan(&dbCountAfterRefetch)
	if dbCountAfterRefetch != 4 {
		t.Errorf("expected 4 rows in DB after gap re-fetch, got %d", dbCountAfterRefetch)
	}
}

func TestInvalidBody(t *testing.T) {
	rr := postHistorical(t, `{}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTransactionRollback(t *testing.T) {
	// Clean slate for this symbol so no cached rows interfere.
	db.Exec(context.Background(), "DELETE FROM market_data WHERE symbol='FAIL'")

	// Mock Alpaca returns bars normally.
	var count atomic.Int32
	srv := mockAlpacaServer(t, &count)
	defer srv.Close()
	alpacaBaseURL = srv.URL

	// Swap in a closed pool to force INSERT failures.
	realDB := db
	closedPool, err := pgxpool.New(context.Background(), "postgres://invalid:invalid@localhost:1/nonexistent?sslmode=disable&connect_timeout=1")
	if err == nil {
		closedPool.Close()
		db = closedPool
	}
	defer func() { db = realDB }()

	body := `{"symbols":["FAIL"],"start_date":"2024-01-02","end_date":"2024-01-05","resolution":"1d"}`
	rr := postHistorical(t, body)

	// With a broken DB the handler should respond 200 (partial-success path) but bars_ready=0,
	// because fetchAndCacheBars logs the error and continues. Verify no rows were committed.
	db = realDB
	var dbCount int
	db.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM market_data WHERE symbol='FAIL'",
	).Scan(&dbCount)
	if dbCount != 0 {
		t.Errorf("expected 0 rows in DB after rollback, got %d (status=%d)", dbCount, rr.Code)
	}
}
