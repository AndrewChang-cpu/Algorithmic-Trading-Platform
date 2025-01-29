package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"

	"github.com/Shopify/sarama"
	"github.com/gorilla/websocket"
)

func main() {
	// Kafka producer configuration

	// // - LOCAL DEV: Load environment variables from .env file
	// producer, err := sarama.NewSyncProducer([]string{"localhost:9092"}, nil)
	// if err != nil {
	// 	log.Fatalf("Error creating Kafka producer: %v", err)
	// }
	// defer producer.Close()

	// err = godotenv.Load()
	// if err != nil {
	// 	log.Fatalf("Error loading .env file")
	// }

	// - CLOUD DEV: Env variables are loaded in as secrets
	producer, err := sarama.NewSyncProducer([]string{"kafka:9092"}, nil)
	if err != nil {
		log.Fatalf("Error creating Kafka producer: %v", err)
	}
	defer producer.Close()

	// Continuously produce messages with stock prices to Kafka topic
	symbols := []string{"GOOG", "AAPL"} // change this to * for all symbols
	subscribeToAlpacaStream(producer, symbols)
	select {} // block forever
}

func subscribeToAlpacaStream(producer sarama.SyncProducer, symbols []string) {
	conn := connectToWebsocket()
	defer conn.Close()

	// Subscribe to stream
	subscribeMessage := map[string]interface{}{
		"action": "subscribe",
		"bars":   symbols,
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
					Key:   sarama.StringEncoder(msg["S"].(string)),
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
