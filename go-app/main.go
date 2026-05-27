package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"application-server/db"
	"application-server/handlers"
	"application-server/middleware"
	"application-server/queue"
	s3client "application-server/s3"

	"github.com/joho/godotenv"
)

func corsMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	origins := map[string]bool{}
	for _, o := range strings.Split(allowedOrigins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins[o] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowedOrigins == "" || origins[origin] || len(origins) == 0 {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// withAuth wraps a handler with JWT authentication.
func withAuth(h http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		middleware.RequireAuth(h).ServeHTTP(w, r)
	})
}

func main() {
	// Load .env for local dev (no-op if missing)
	_ = godotenv.Load()

	// JWT keys
	privKey := os.Getenv("JWT_PRIVATE_KEY_PATH")
	pubKey := os.Getenv("JWT_PUBLIC_KEY_PATH")
	if privKey == "" {
		privKey = "./jwt_private.pem"
	}
	if pubKey == "" {
		pubKey = "./jwt_public.pem"
	}
	if err := middleware.LoadKeys(privKey, pubKey); err != nil {
		log.Fatalf("loading JWT keys: %v", err)
	}

	// Database
	if err := db.Init(os.Getenv("DATABASE_URL")); err != nil {
		log.Fatalf("connecting to database: %v", err)
	}

	// S3 / MinIO
	if err := s3client.Init(
		os.Getenv("S3_ENDPOINT"),
		os.Getenv("S3_ACCESS_KEY"),
		os.Getenv("S3_SECRET_KEY"),
		os.Getenv("S3_BUCKET"),
		os.Getenv("S3_REGION"),
	); err != nil {
		log.Fatalf("initializing S3: %v", err)
	}

	// Redis / Celery queue
	if err := queue.Init(os.Getenv("REDIS_URL")); err != nil {
		log.Fatalf("connecting to Redis: %v", err)
	}

	mux := http.NewServeMux()

	// Public auth routes
	mux.HandleFunc("POST /api/auth/register", handlers.Register)
	mux.HandleFunc("POST /api/auth/login", handlers.Login)
	mux.HandleFunc("POST /api/auth/refresh", handlers.Refresh)
	mux.HandleFunc("POST /api/auth/logout", handlers.Logout)

	// Health check (public)
	mux.HandleFunc("GET /api/health", handlers.HealthCheck)

	// Strategies (protected)
	mux.HandleFunc("GET /api/strategies", withAuth(handlers.ListStrategies))
	mux.HandleFunc("POST /api/strategies", withAuth(handlers.UploadStrategy))
	mux.HandleFunc("GET /api/strategies/{id}", withAuth(handlers.GetStrategy))
	mux.HandleFunc("POST /api/strategies/{id}/versions", withAuth(handlers.UploadNewVersion))
	mux.HandleFunc("GET /api/strategies/{id}/versions/{versionId}/code", withAuth(handlers.GetVersionCode))
	mux.HandleFunc("DELETE /api/strategies/{id}", withAuth(handlers.DeleteStrategy))

	// Jobs (protected)
	mux.HandleFunc("GET /api/jobs", withAuth(handlers.ListJobs))
	mux.HandleFunc("POST /api/jobs", withAuth(handlers.SubmitJob))
	mux.HandleFunc("GET /api/jobs/{id}", withAuth(handlers.GetJob))
	mux.HandleFunc("GET /api/jobs/{id}/metrics", withAuth(handlers.GetJobMetrics))
	mux.HandleFunc("GET /api/jobs/{id}/portfolio", withAuth(handlers.GetPortfolio))
	mux.HandleFunc("POST /api/jobs/{id}/cancel", withAuth(handlers.CancelJob))

	// WebSocket streams (auth via ?token= query param, handled inside handler)
	mux.HandleFunc("GET /api/stream/jobs/{id}", handlers.JobStatusStream)
	mux.HandleFunc("GET /api/stream/portfolio/{jobId}", handlers.PortfolioStream)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cors := corsMiddleware(os.Getenv("CORS_ORIGINS"))
	log.Printf("go-app listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, cors(mux)))
}
