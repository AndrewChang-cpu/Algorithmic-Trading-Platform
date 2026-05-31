package models

import "time"

// DB entities

type User struct {
	ID           string    `json:"id" db:"id"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time `json:"updatedAt" db:"updated_at"`
}

type Strategy struct {
	ID          string    `json:"id" db:"id"`
	UserID      string    `json:"userId" db:"user_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

type StrategyVersion struct {
	ID            string    `json:"id" db:"id"`
	StrategyID    string    `json:"strategyId" db:"strategy_id"`
	VersionNumber int       `json:"versionNumber" db:"version_number"`
	S3Key         string    `json:"s3Key" db:"s3_key"`
	CreatedAt     time.Time `json:"createdAt" db:"created_at"`
}

type Job struct {
	ID                string     `json:"id" db:"id"`
	UserID            string     `json:"userId" db:"user_id"`
	StrategyVersionID string     `json:"strategyVersionId" db:"strategy_version_id"`
	Type              string     `json:"type" db:"type"`
	Status            string     `json:"status" db:"status"`
	DataSource        string     `json:"dataSource" db:"data_source"`
	Symbols           []string   `json:"symbols" db:"symbols"`
	Resolution        string     `json:"resolution" db:"resolution"`
	StartDate         *string    `json:"startDate,omitempty" db:"start_date"`
	EndDate           *string    `json:"endDate,omitempty" db:"end_date"`
	WarmupDays        *int       `json:"warmupDays,omitempty" db:"warmup_days"`
	CsvS3Key          *string    `json:"csvS3Key,omitempty" db:"csv_s3_key"`
	ErrorMessage      *string    `json:"errorMessage,omitempty" db:"error_message"`
	TimeoutSeconds    int        `json:"timeoutSeconds" db:"timeout_seconds"`
	CreatedAt         time.Time  `json:"createdAt" db:"created_at"`
	StartedAt         *time.Time `json:"startedAt,omitempty" db:"started_at"`
	CompletedAt       *time.Time `json:"completedAt,omitempty" db:"completed_at"`
}

type JobLog struct {
	ID        int       `json:"id" db:"id"`
	JobID     string    `json:"jobId" db:"job_id"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	Level     string    `json:"level" db:"level"`
	Message   string    `json:"message" db:"message"`
}

type RefreshToken struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"userId" db:"user_id"`
	TokenHash string    `json:"-" db:"token_hash"`
	ExpiresAt time.Time `json:"expiresAt" db:"expires_at"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// Request types

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SubmitJobRequest struct {
	StrategyVersionID string   `json:"strategyVersionId"`
	Type              string   `json:"type"`
	DataSource        string   `json:"dataSource"`
	Symbols           []string `json:"symbols"`
	Resolution        string   `json:"resolution"`
	StartDate         string   `json:"startDate,omitempty"`
	EndDate           string   `json:"endDate,omitempty"`
	WarmupDays        int      `json:"warmupDays,omitempty"`
}

// Response types

type AuthResponse struct {
	AccessToken string `json:"accessToken"`
	UserID      string `json:"userId,omitempty"`
}

type StrategyResponse struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	LatestVersion int                       `json:"latestVersion"`
	RunCount      int                       `json:"runCount"`
	BestSharpe    *float64                  `json:"bestSharpe"`
	CreatedAt     time.Time                 `json:"createdAt"`
	Versions      []StrategyVersionResponse `json:"versions,omitempty"`
	Stats         *StrategyStats            `json:"stats,omitempty"`
}

type StrategyVersionResponse struct {
	ID            string    `json:"id"`
	VersionNumber int       `json:"versionNumber"`
	CreatedAt     time.Time `json:"createdAt"`
}

type StrategyStats struct {
	RunCount   int      `json:"runCount"`
	BestSharpe *float64 `json:"bestSharpe"`
	AvgReturn  *float64 `json:"avgReturn"`
}

type JobResponse struct {
	ID            string     `json:"id"`
	StrategyName  string     `json:"strategyName"`
	VersionNumber int        `json:"versionNumber"`
	Type          string     `json:"type"`
	Status        string     `json:"status"`
	DataSource    string     `json:"dataSource"`
	Symbols       []string   `json:"symbols"`
	Resolution    string     `json:"resolution"`
	ErrorMessage  *string    `json:"errorMessage,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

type JobListResponse struct {
	Jobs  []JobResponse `json:"jobs"`
	Total int           `json:"total"`
}

type PortfolioPoint struct {
	Time  time.Time `json:"time"`
	Open  float64   `json:"open"`
	High  float64   `json:"high"`
	Low   float64   `json:"low"`
	Close float64   `json:"close"`
}

type PortfolioResponse struct {
	Points []PortfolioPoint `json:"points"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
