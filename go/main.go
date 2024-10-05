package main

import (
	"log"
	"net/url"
	"os"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// Read API key and secret from environment variables
	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")

	// TEST STREAM
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
	defer conn.Close()

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

	// TEST STREAM SYMBOL
	// Subscribe to stream
	subscribeMessage := map[string]interface{}{
		"action": "subscribe",
		"bars":   []string{"FAKEPACA"}, // change this to * for all symbols
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
	}
}
