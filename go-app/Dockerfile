# Stage 1: Build the application
FROM golang:1.23.2-alpine AS builder

WORKDIR /app

# - Download ownload dependencies first
COPY go.mod go.sum ./

# - Download and cache Go modules
RUN go mod download

# - Copy the rest of the application source code
COPY . .

RUN go build -o main .

# Stage 2: Create a minimal runtime image
FROM alpine:latest

# - Install CA certificates for HTTPS if needed
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/main .

# Expose the application port
EXPOSE 8080
CMD ["./main"]