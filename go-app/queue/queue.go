package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

var rdb *redis.Client

func Init(redisURL string) error {
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("parsing Redis URL: %w", err)
	}
	rdb = redis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("connecting to Redis: %w", err)
	}
	return nil
}

// celeryMessage builds a Celery protocol v1 task message.
func celeryMessage(taskName, jobID string) []byte {
	msg := map[string]interface{}{
		"id":      uuid.NewString(),
		"task":    taskName,
		"args":    []interface{}{jobID},
		"kwargs":  map[string]interface{}{},
		"retries": 0,
		"eta":     nil,
		"expires": nil,
	}
	data, _ := json.Marshal(msg)
	return data
}

func EnqueueBacktest(jobID string) error {
	msg := celeryMessage("atp.run_lean_backtest", jobID)
	return rdb.RPush(context.Background(), "celery", msg).Err()
}

func EnqueueLive(jobID string) error {
	msg := celeryMessage("atp.run_lean_live", jobID)
	return rdb.RPush(context.Background(), "celery", msg).Err()
}

func SetStopSignal(jobID string) error {
	return rdb.Set(context.Background(),
		fmt.Sprintf("job:%s:stop", jobID),
		"1",
		time.Hour,
	).Err()
}

func GetClient() *redis.Client {
	return rdb
}
