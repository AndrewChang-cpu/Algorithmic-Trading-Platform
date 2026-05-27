package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Shopify/sarama"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

var db *pgxpool.Pool

var alpacaBaseURL = "https://data.alpaca.markets"

// HistoricalRequest is the body for POST /data/historical
type HistoricalRequest struct {
	Symbols    []string `json:"symbols"`
	StartDate  string   `json:"start_date"`
	EndDate    string   `json:"end_date"`
	Resolution string   `json:"resolution"` // "1d", "1h", "1m"
}

// AlpacaBar matches the bar fields from Alpaca's REST API response
type AlpacaBar struct {
	Timestamp string  `json:"t"`
	Open      float64 `json:"o"`
	High      float64 `json:"h"`
	Low       float64 `json:"l"`
	Close     float64 `json:"c"`
	Volume    int64   `json:"v"`
}

// alpacaTimeframe converts our resolution to Alpaca's timeframe param
func alpacaTimeframe(resolution string) string {
	switch resolution {
	case "1h":
		return "1Hour"
	case "1m":
		return "1Min"
	default:
		return "1Day"
	}
}

func fetchAndCacheBars(ctx context.Context, symbol, startDate, endDate, resolution string) (int, error) {
	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")
	timeframe := alpacaTimeframe(resolution)

	// Check existing coverage: count rows in market_data for this symbol/resolution/range
	var existingCount int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM market_data WHERE symbol=$1 AND resolution=$2 AND time >= $3::date AND time <= $4::date`,
		symbol, resolution, startDate, endDate,
	).Scan(&existingCount)
	if err != nil {
		return 0, fmt.Errorf("coverage check failed: %w", err)
	}

	if existingCount > 0 {
		return existingCount, nil
	}

	// Fetch from Alpaca REST API
	apiURL := fmt.Sprintf(
		"%s/v2/stocks/%s/bars?timeframe=%s&start=%sT00:00:00Z&end=%sT23:59:59Z&limit=10000&adjustment=raw",
		alpacaBaseURL, symbol, timeframe, startDate, endDate,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return existingCount, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("APCA-API-KEY-ID", apiKey)
	req.Header.Set("APCA-API-SECRET-KEY", apiSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return existingCount, fmt.Errorf("alpaca request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return existingCount, fmt.Errorf("alpaca returned %d", resp.StatusCode)
	}

	var body struct {
		Bars []AlpacaBar `json:"bars"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return existingCount, fmt.Errorf("decoding alpaca response: %w", err)
	}

	if len(body.Bars) == 0 {
		return existingCount, nil
	}

	// Bulk insert into market_data
	tx, err := db.Begin(ctx)
	if err != nil {
		return existingCount, err
	}
	defer tx.Rollback(ctx)

	for _, bar := range body.Bars {
		t, err := time.Parse(time.RFC3339, bar.Timestamp)
		if err != nil {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO market_data (time, symbol, resolution, open, high, low, close, volume)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT DO NOTHING
		`, t, symbol, resolution, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume)
		if err != nil {
			log.Printf("insert error for %s@%s: %v", symbol, bar.Timestamp, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return existingCount, err
	}

	// Return total count after insert
	var totalCount int
	db.QueryRow(ctx,
		`SELECT COUNT(*) FROM market_data WHERE symbol=$1 AND resolution=$2 AND time >= $3::date AND time <= $4::date`,
		symbol, resolution, startDate, endDate,
	).Scan(&totalCount)

	return totalCount, nil
}

func handleHistorical(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if db == nil {
		http.Error(w, `{"error":"database not connected"}`, http.StatusServiceUnavailable)
		return
	}

	var req HistoricalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.Symbols) == 0 || req.StartDate == "" || req.EndDate == "" || req.Resolution == "" {
		http.Error(w, `{"error":"symbols, start_date, end_date, resolution are required"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	totalBars := 0

	for _, symbol := range req.Symbols {
		count, err := fetchAndCacheBars(ctx, symbol, req.StartDate, req.EndDate, req.Resolution)
		if err != nil {
			log.Printf("fetchAndCacheBars error for %s: %v", symbol, err)
			// Continue with other symbols; partial success is ok
		}
		totalBars += count
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"bars_ready": totalBars})
}

func main() {
	// Load .env for local dev
	_ = godotenv.Load()

	if v := os.Getenv("ALPACA_BASE_URL"); v != "" {
		alpacaBaseURL = v
	}

	// Init PostgreSQL pool
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		pool, err := pgxpool.New(context.Background(), dbURL)
		if err != nil {
			log.Printf("Warning: could not connect to DB: %v (historical endpoint will not work)", err)
		} else {
			db = pool
			defer db.Close()
			log.Println("Connected to PostgreSQL")
		}
	}

	// Kafka producer
	kafkaBrokers := os.Getenv("KAFKA_BOOTSTRAP_SERVERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
	}

	producer, err := sarama.NewSyncProducer([]string{kafkaBrokers}, nil)
	if err != nil {
		log.Fatalf("Error creating Kafka producer: %v", err)
	}
	defer producer.Close()

	// HTTP server for internal historical data endpoint
	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8081"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/data/historical", handleHistorical)
	go func() {
		log.Printf("go-data HTTP server on :%s", httpPort)
		if err := http.ListenAndServe(":"+httpPort, mux); err != nil {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Start Alpaca WebSocket -> Kafka producer (existing logic)
	symbols := []string{"SPY"}
	if sym := os.Getenv("ALPACA_SYMBOLS"); sym != "" {
		symbols = []string{sym}
	}
	go subscribeToAlpacaStream(producer, symbols)

	log.Println("go-data running")
	select {} // block forever
}

func subscribeToAlpacaStream(producer sarama.SyncProducer, symbols []string) {
	conn := connectToWebsocket()
	if conn == nil {
		log.Println("WebSocket connection failed, skipping live stream")
		return
	}
	defer conn.Close()

	subscribeMessage := map[string]interface{}{
		"action": "subscribe",
		"bars":   symbols,
	}
	if err := conn.WriteJSON(subscribeMessage); err != nil {
		log.Printf("Subscription error: %v", err)
		return
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}

		var messages []map[string]interface{}
		if err := json.Unmarshal(message, &messages); err != nil {
			continue
		}

		for _, msg := range messages {
			if msg["T"] == "b" {
				kafkaMessage := &sarama.ProducerMessage{
					Topic: "stock_data",
					Key:   sarama.StringEncoder(fmt.Sprintf("%v", msg["S"])),
					Value: sarama.StringEncoder(string(message)),
				}
				partition, offset, err := producer.SendMessage(kafkaMessage)
				if err != nil {
					log.Printf("Kafka send error: %v", err)
				} else {
					log.Printf("Sent to partition %d offset %d", partition, offset)
				}
			}
		}
	}
}

func connectToWebsocket() *websocket.Conn {
	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		log.Println("ALPACA_API_KEY/SECRET not set, skipping WebSocket connection")
		return nil
	}

	socketURL := url.URL{
		Scheme: "wss",
		Host:   "stream.data.alpaca.markets",
		Path:   "/v2/iex",
	}

	conn, _, err := websocket.DefaultDialer.Dial(socketURL.String(), nil)
	if err != nil {
		log.Printf("WebSocket dial error: %v", err)
		return nil
	}

	authMessage := map[string]string{
		"action": "auth",
		"key":    apiKey,
		"secret": apiSecret,
	}
	if err := conn.WriteJSON(authMessage); err != nil {
		log.Printf("Auth error: %v", err)
		conn.Close()
		return nil
	}

	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Auth read error: %v", err)
		conn.Close()
		return nil
	}
	log.Printf("WebSocket auth response: %s", message)
	return conn
}
