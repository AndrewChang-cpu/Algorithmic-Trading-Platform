package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-redis/redis/v8"
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

func main() {
	redisClient := initRedisClient()
	defer redisClient.Close()

	http.HandleFunc("/publish_task", func(w http.ResponseWriter, r *http.Request) {
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
	})

	fmt.Println("Go server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
