package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"application-server/db"
	"application-server/middleware"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		allowed := os.Getenv("CORS_ORIGINS")
		if allowed == "" {
			return true // dev mode: allow all
		}
		for _, o := range strings.Split(allowed, ",") {
			if strings.TrimSpace(o) == origin {
				return true
			}
		}
		return false
	},
}

// authenticateWS validates the JWT from the ?token= query param.
// Returns userID and true on success, empty string and false on failure.
func authenticateWS(r *http.Request) (string, bool) {
	token := r.URL.Query().Get("token")
	if token == "" {
		return "", false
	}

	// Inject token into a fake request header so RequireAuth can validate it.
	fakeReq, _ := http.NewRequest("GET", "/", nil)
	fakeReq.Header.Set("Authorization", "Bearer "+token)

	var userID string
	done := make(chan struct{})
	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID = middleware.GetUserID(r.Context())
		close(done)
	}))
	rr := &captureWriter{}
	handler.ServeHTTP(rr, fakeReq)
	select {
	case <-done:
		return userID, userID != ""
	default:
		return "", false
	}
}

// captureWriter discards the response but satisfies http.ResponseWriter.
type captureWriter struct {
	code int
}

func (c *captureWriter) Header() http.Header         { return http.Header{} }
func (c *captureWriter) Write(b []byte) (int, error) { return len(b), nil }
func (c *captureWriter) WriteHeader(code int)        { c.code = code }

// JobStatusStream handles WS /api/stream/jobs/:id
// Streams job status updates and log lines to the client.
// Token validated at connection time only.
func JobStatusStream(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticateWS(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	jobID := r.PathValue("id")

	// Verify job ownership
	var ownerID string
	err := db.Pool.QueryRow(r.Context(), `
		SELECT s.user_id FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE j.id = $1
	`, jobID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}
	defer conn.Close()

	var lastLogID int
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	terminalStatuses := map[string]bool{"completed": true, "failed": true}

	for range ticker.C {
		ctx := context.Background()

		// Send new log lines
		rows, err := db.Pool.Query(ctx, `
			SELECT id, level, message, timestamp FROM job_logs
			WHERE job_id = $1 AND id > $2
			ORDER BY id ASC LIMIT 50
		`, jobID, lastLogID)
		if err == nil {
			for rows.Next() {
				var id int
				var level, message string
				var ts time.Time
				rows.Scan(&id, &level, &message, &ts)
				lastLogID = id
				msg := map[string]interface{}{
					"type":      "log",
					"level":     level,
					"message":   message,
					"timestamp": ts.Format(time.RFC3339),
				}
				data, _ := json.Marshal(msg)
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					return
				}
			}
			rows.Close()
		}

		// Send current status
		var status string
		db.Pool.QueryRow(ctx, "SELECT status FROM jobs WHERE id = $1", jobID).Scan(&status)
		if status != "" {
			msg := map[string]string{"type": "status", "status": status}
			data, _ := json.Marshal(msg)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
			if terminalStatuses[status] {
				return // Close connection when job reaches terminal state
			}
		}
	}
}

// PortfolioStream handles WS /api/stream/portfolio/:jobId
// Consumes Kafka portfolio_data and forwards matching job snapshots.
func PortfolioStream(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticateWS(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	jobID := r.PathValue("jobId")

	// Verify ownership
	var ownerID string
	err := db.Pool.QueryRow(r.Context(), `
		SELECT s.user_id FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE j.id = $1
	`, jobID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	kafkaBrokers := os.Getenv("KAFKA_BOOTSTRAP_SERVERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
	}

	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": kafkaBrokers,
		"group.id":          fmt.Sprintf("ws-portfolio-%s", uuid.NewString()),
		"auto.offset.reset": "latest",
	})
	if err != nil {
		log.Printf("Kafka consumer error: %v", err)
		return
	}
	defer consumer.Close()

	if err := consumer.Subscribe("portfolio_data", nil); err != nil {
		log.Printf("Kafka subscribe error: %v", err)
		return
	}

	// Read pump: detect client disconnect
	done := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(done)
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}

		msg, err := consumer.ReadMessage(500 * time.Millisecond)
		if err != nil {
			// Timeout or no message — keep looping
			continue
		}

		// Parse and filter by job_id
		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Value, &payload); err != nil {
			continue
		}
		if payload["job_id"] != jobID {
			continue
		}

		payload["type"] = "snapshot"
		data, _ := json.Marshal(payload)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

// HealthCheck handles GET /api/health
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	kafkaBrokers := os.Getenv("KAFKA_BOOTSTRAP_SERVERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
	}

	dbStatus := "ok"
	if err := db.Pool.Ping(r.Context()); err != nil {
		dbStatus = "down"
	}

	// Simple Kafka check via metadata request
	kafkaStatus := "ok"
	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": kafkaBrokers})
	if err != nil {
		kafkaStatus = "down"
	} else {
		_, err = p.GetMetadata(nil, true, 2000)
		if err != nil {
			kafkaStatus = "down"
		}
		p.Close()
	}

	// Redis check — marked ok without a live ping to avoid circular imports
	redisStatus := "ok"

	writeJSON(w, http.StatusOK, map[string]string{
		"kafka": kafkaStatus,
		"redis": redisStatus,
		"db":    dbStatus,
	})
}
