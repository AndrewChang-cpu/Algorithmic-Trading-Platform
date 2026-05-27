FROM golang:1.23 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN apt-get update && apt-get install -y librdkafka-dev
RUN go build -o server .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y librdkafka1 ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /app/server .
EXPOSE 8080
CMD ["./server"]
