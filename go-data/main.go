package main

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"os"

	"github.com/Shopify/sarama"
	"github.com/gorilla/websocket"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	// Kafka producer configuration
	producer, err := sarama.NewSyncProducer([]string{"kafka:9092"}, nil)
	if (err != nil) {
		log.Fatalf("Error creating Kafka producer: %v", err)
	}
	defer producer.Close()

	// LOCAL DEV: Load environment variables from .env file
	// err = godotenv.Load()
	// if err != nil {
	// 	log.Fatalf("Error loading .env file")
	// }

	// Continuously produce messages with stock prices to Kafka topic
	symbols := [...]string{"FAKEPACA"} // change this to * for all symbols
	for _, symbol := range symbols {
		go subscribeToStream(producer, symbol)
	}
	select {} // block forever
}

func subscribeToStream(producer sarama.SyncProducer, symbol string) {
	return
	conn := connectToWebsocket()
	defer conn.Close()

	// Subscribe to stream
	subscribeMessage := map[string]interface{}{
		"action": "subscribe",
		"bars":   []string{symbol},
	}
	if err := conn.WriteJSON(subscribeMessage); err != nil {
		log.Fatalf("Subscription error: %v", err)
	}

	// Continuously listen for messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}
		log.Printf("Received: %s", message)

		// Parse the message to check if "T" is "b"
		var messages []map[string]interface{}
		if err := json.Unmarshal(message, &messages); err != nil {
			log.Printf("Error unmarshalling message: %v", err)
			continue
		}

		for _, msg := range messages {
			if msg["T"] == "b" {
				// Produce the WebSocket message to Kafka
				kafkaMessage := &sarama.ProducerMessage{
					Topic: "stock_data",
					Key:   sarama.StringEncoder("S"),
					Value: sarama.StringEncoder(string(message)),
				}

				partition, offset, err := producer.SendMessage(kafkaMessage)
				if err != nil {
					log.Printf("Error sending message to Kafka: %v", err)
				} else {
					log.Printf("Message sent to partition %d at offset %d", partition, offset)
				}
			}
		}
	}
}

func connectToWebsocket() *websocket.Conn {
	// Read API key and secret from environment variables
	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")

	// Alpaca WebSocket URL for the stream
	socketURL := url.URL{
		Scheme: "wss",
		Host:   "stream.data.alpaca.markets",
		Path:   "/v2/test",
	}

	// Connect to the WebSocket
	log.Printf("Connecting to %s", socketURL.String())
	conn, _, err := websocket.DefaultDialer.Dial(socketURL.String(), nil)
	if err != nil {
		log.Fatalf("Error connecting to WebSocket: %v", err)
	}

	// Authenticate with the WebSocket
	authMessage := map[string]string{
		"action": "auth",
		"key":    apiKey,
		"secret": apiSecret,
	}
	if err := conn.WriteJSON(authMessage); err != nil {
		log.Fatalf("Authentication error: %v", err)
	}

	// Listen for authentication success
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("Error reading message: %v", err)
	}
	log.Printf("Received: %s", message)

	return conn
}
