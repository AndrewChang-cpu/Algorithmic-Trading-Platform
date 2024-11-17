package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var ctx = context.Background()

// TaskData represents the data structure for the task with only the "stake" parameter
type TaskData struct {
	Stake int `json:"stake"`
}

// Initialize Redis client
func initRedisClient() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // Redis server address
		Password: "",               // no password set
		DB:       0,                // use default DB
	})

	_, err := client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	return client
}
func handlePublishTask(w http.ResponseWriter, r *http.Request, redisClient *redis.Client) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the JSON request body
	var task TaskData
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Convert task data to JSON to publish to Redis
	taskData, err := json.Marshal(task)
	if err != nil {
		http.Error(w, "Could not encode task data", http.StatusInternalServerError)
		return
	}

	// Publish to the Redis queue
	err = redisClient.RPush(ctx, "celery", taskData).Err()
	if err != nil {
		http.Error(w, "Failed to publish task", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Task published successfully")
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for testing
	},
}

func handlePortfolioWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	// Create Kafka consumer
	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"group.id":          "portfolio-websocket-group",
		"auto.offset.reset": "latest",
	})
	if err != nil {
		log.Printf("Failed to create consumer: %v", err)
		return
	}
	defer consumer.Close()

	// Subscribe to portfolio_data topic
	err = consumer.Subscribe("portfolio_data", nil)
	if err != nil {
		log.Printf("Failed to subscribe to topic: %v", err)
		return
	}

	// Continue reading messages until connection is closed
	for {
		msg, err := consumer.ReadMessage(-1)
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}

		// Forward message to WebSocket
		if err := conn.WriteMessage(websocket.TextMessage, msg.Value); err != nil {
			log.Printf("Error writing message: %v", err)
			break
		}
	}
}

func main() {
	redisClient := initRedisClient()
	defer redisClient.Close()

	http.HandleFunc("/publish_task", func(w http.ResponseWriter, r *http.Request) {
		handlePublishTask(w, r, redisClient)
	})
	http.HandleFunc("/portfolio_stream", handlePortfolioWebSocket)

	fmt.Println("Go server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
