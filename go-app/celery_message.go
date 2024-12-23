// This file is jerry-rigged to publish a message to the Redis queue in Go
// The decision to use Celery/Python in the first place was because Backtrader has a lot of great functionality you don't get from Go libraries
// This is the easiest point in the code to transition from Go to Python. Modify in the future if needed.

package main

import (
	"encoding/base64"
	"encoding/json"
	"log"

	"github.com/google/uuid"
)

// CeleryMessage represents the entire structure of a Celery task message
type CeleryMessage struct {
	Body            string           `json:"body"`
	ContentEncoding string           `json:"content-encoding"`
	ContentType     string           `json:"content-type"`
	Headers         CeleryHeaders    `json:"headers"`
	Properties      CeleryProperties `json:"properties"`
}

// CeleryHeaders represents the headers in the Celery message
type CeleryHeaders struct {
	Lang                string                 `json:"lang"`
	Task                string                 `json:"task"`
	ID                  string                 `json:"id"`
	Shadow              *string                `json:"shadow"`
	Eta                 *string                `json:"eta"`
	Expires             *string                `json:"expires"`
	Group               *string                `json:"group"`
	GroupIndex          *string                `json:"group_index"`
	Retries             int                    `json:"retries"`
	TimeLimit           [2]*string             `json:"timelimit"`
	RootID              string                 `json:"root_id"`
	ParentID            *string                `json:"parent_id"`
	ArgsRepr            string                 `json:"argsrepr"`
	KwargsRepr          string                 `json:"kwargsrepr"`
	Origin              string                 `json:"origin"`
	IgnoreResult        bool                   `json:"ignore_result"`
	ReplacedTaskNesting int                    `json:"replaced_task_nesting"`
	Stamps              map[string]interface{} `json:"stamps"`
}

// CeleryProperties represents the properties in the Celery message
type CeleryProperties struct {
	CorrelationID string                 `json:"correlation_id"`
	ReplyTo       string                 `json:"reply_to"`
	DeliveryMode  int                    `json:"delivery_mode"`
	DeliveryInfo  map[string]interface{} `json:"delivery_info"`
	Priority      int                    `json:"priority"`
	BodyEncoding  string                 `json:"body_encoding"`
	DeliveryTag   string                 `json:"delivery_tag"`
}

func ConstructMessage() []byte { // TaskParams params
	// Define task arguments
	args := []interface{}{"foo"}       // Positional arguments
	kwargs := map[string]interface{}{} // Keyword arguments
	embed := map[string]interface{}{}  // Embedded data (usually empty)

	// Serialize the body as JSON and Base64 encode it
	bodyData, err := json.Marshal([]interface{}{args, kwargs, embed})
	if err != nil {
		log.Fatalf("Failed to serialize task body: %v", err)
	}
	body := base64.StdEncoding.EncodeToString(bodyData)

	// Generate unique IDs
	taskID := uuid.NewString()
	replyToID := uuid.NewString()

	// Construct the Celery message
	message := CeleryMessage{
		Body:            body,
		ContentEncoding: "utf-8",
		ContentType:     "application/json",
		Headers: CeleryHeaders{
			Lang:                "py",
			Task:                "celery_worker.run_test_strategy",
			ID:                  taskID,
			Shadow:              nil,
			Eta:                 nil,
			Expires:             nil,
			Group:               nil,
			GroupIndex:          nil,
			Retries:             0,
			TimeLimit:           [2]*string{nil, nil},
			RootID:              taskID,
			ParentID:            nil,
			ArgsRepr:            "['foo']",
			KwargsRepr:          "{}",
			Origin:              "gen29460@ImStupid",
			IgnoreResult:        false,
			ReplacedTaskNesting: 0,
			Stamps:              map[string]interface{}{},
		},
		Properties: CeleryProperties{
			CorrelationID: taskID,
			ReplyTo:       replyToID,
			DeliveryMode:  2,
			DeliveryInfo: map[string]interface{}{
				"exchange":    "",
				"routing_key": "celery",
			},
			Priority:     0,
			BodyEncoding: "base64",
			DeliveryTag:  uuid.NewString(),
		},
	}

	// Serialize the message to JSON
	messageJSON, err := json.Marshal(message)
	if err != nil {
		log.Fatalf("Failed to serialize Celery message: %v", err)
	}

	return messageJSON
}
