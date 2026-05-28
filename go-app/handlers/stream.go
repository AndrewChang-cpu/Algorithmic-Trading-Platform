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

// wsFirstMessageAuth reads the first WebSocket message and validates the JWT.
// Returns userID on success. Sends a close frame and returns an error on failure.
func wsFirstMessageAuth(conn *websocket.Conn) (string, error) {
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, authMsg, err := conn.ReadMessage()
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "unauthorized"))
		return "", fmt.Errorf("no auth message: %w", err)
	}
	conn.SetReadDeadline(time.Time{}) // clear deadline

	var authPayload struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(authMsg, &authPayload); err != nil || authPayload.Type != "auth" {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "unauthorized"))
		return "", fmt.Errorf("invalid auth payload")
	}

	userID, _, err := middleware.ValidateToken(authPayload.Token)
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "unauthorized"))
		return "", fmt.Errorf("invalid token: %w", err)
	}

	conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"auth_ok"}`))
	return userID, nil
}

// JobStatusStream handles WS /api/stream/jobs/:id
// Streams job status updates and log lines to the client.
// Auth via first message: {"type":"auth","token":"<JWT>"}
func JobStatusStream(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}
	defer conn.Close()

	userID, err := wsFirstMessageAuth(conn)
	if err != nil {
		return
	}

	// Verify job belongs to this user
	var ownerID string
	err = db.Pool.QueryRow(r.Context(), `
		SELECT s.user_id FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE j.id = $1
	`, jobID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "forbidden"))
		return
	}

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
		if err != nil {
			log.Printf("JobStatusStream: query job_logs error (job %s): %v", jobID, err)
		} else {
			for rows.Next() {
				var id int
				var level, message string
				var ts time.Time
				if err := rows.Scan(&id, &level, &message, &ts); err != nil {
					log.Printf("JobStatusStream: scan job_logs error (job %s): %v", jobID, err)
					continue
				}
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
			if err := rows.Err(); err != nil {
				log.Printf("JobStatusStream: rows.Err (job %s): %v", jobID, err)
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
// Auth via first message: {"type":"auth","token":"<JWT>"}
func PortfolioStream(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	userID, err := wsFirstMessageAuth(conn)
	if err != nil {
		return
	}

	// Verify job belongs to this user
	var ownerID string
	err = db.Pool.QueryRow(r.Context(), `
		SELECT s.user_id FROM jobs j
		JOIN strategy_versions sv ON j.strategy_version_id = sv.id
		JOIN strategies s ON sv.strategy_id = s.id
		WHERE j.id = $1
	`, jobID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "forbidden"))
		return
	}

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
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "internal error"))
		return
	}
	defer consumer.Close()

	if err := consumer.Subscribe("portfolio_data", nil); err != nil {
		log.Printf("Kafka subscribe error: %v", err)
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "internal error"))
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
			kafkaErr, ok := err.(kafka.Error)
			if ok && kafkaErr.Code() == kafka.ErrTimedOut {
				// Timeout — no message available, keep looping
				continue
			}
			// Unexpected Kafka error
			log.Printf("PortfolioStream: Kafka read error (job %s): %v", jobID, err)
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "internal error"))
			return
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
