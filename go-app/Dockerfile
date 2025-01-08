# Stage 1: Build the application
FROM golang:1.23.2 AS builder

WORKDIR /app

# - Download ownload dependencies first
COPY go.mod go.sum ./

# - Download and cache Go modules
RUN go mod download

# - Copy the rest of the application source code
COPY . .

# Build the Go application
RUN go build -o main .

# Stage 2: Create a minimal runtime image
FROM ubuntu:latest

# Install CA certificates and necessary libraries for Kafka
RUN apt-get update && apt-get install -y ca-certificates librdkafka1 && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/main .

# Expose the application port
EXPOSE 8080

CMD ["./main"]