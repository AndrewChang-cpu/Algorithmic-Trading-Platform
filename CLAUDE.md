**Overview**
- **Purpose**: This repository is an algorithmic trading platform prototype. The goal is for users to upload Python strategies, live or backtests, and monitor the progress. It ingests market data (Alpaca), publishes per-stock data to Kafka topics, and runs Backtrader strategies executed by Celery workers that consume the Kafka topics. The frontend queries/dispatches strategy runs and observed state is published back to Kafka.

**Architecture**
- **Data producers**: Alpaca historical/live data (research notebooks and download scripts live under [research](research)). Producers should publish stock bars to the `stock_data` Kafka topic.
- **Message broker**: Kafka for market data topics; Redis for Celery broker/back-end.
- **Consumers / Strategies**: Backtrader runs inside Celery workers and reads live bars via the custom `KafkaDataFeed` in [python/strategy.py](python/strategy.py).
- **Orchestration**: Local quick-start uses [local-kafka-docker-compose.yml](local-kafka-docker-compose.yml). Cloud deployment uses Kubernetes via [kops.yaml](kops.yaml) and manifests under the [kubernetes](kubernetes) folder.
- **Frontend**: UI and orchestration client in the `web/` folder (React + Vite).

**Key files**
- **Kafka compose**: [local-kafka-docker-compose.yml](local-kafka-docker-compose.yml)
- **Kops cluster config**: [kops.yaml](kops.yaml)
- **K8s helper**: [kubernetes/initialize.sh](kubernetes/initialize.sh)
- **Celery worker**: [python/celery_worker.py](python/celery_worker.py)
- **API to run strategies**: [go-app/main.go](go-app/main.go)
- **Data publisher**: [go-data/main.go](go-data/main.go)

**Local development (quick start)**
Prereqs: `docker` + `docker compose` (daemon running), Python 3.10+, `pip`, Go 1.23+, Node/npm (for the frontend), and a Python virtual environment tool.

1. Start infrastructure (Kafka, Zookeeper, Redis):

   - Run:
     - `docker compose -f local-kafka-docker-compose.yml up -d`

   - What this provides:
     - Zookeeper: `2181`
     - Kafka broker: `9092` (advertised OUTSIDE)
     - Redis: `6379`

2. (Optional) Configure Alpaca streaming credentials for `go-data`:
    - Create a `.env` file in `go-data/` with:
      - `ALPACA_API_KEY=...`
      - `ALPACA_API_SECRET=...`

3. Run Go services (data publishing and strategy scheduling API)
    - `cd go-app && go run .`
    - `cd go-data && go run main.go` (requires Alpaca keys and network access; otherwise skip and use the test producer below)

4. Install Python deps:
    - `pip install -r requirements.txt`

5. Start Celery worker (local dev):

   - From the `python/` folder:
     - `cd python`
     - `celery -A celery_worker worker --loglevel=info`

6. Run a test consumer or strategy:

   - Consumer test (prints messages it reads):
     - `cd python && python consumer_test.py`

   - Run the Backtrader strategy locally (consumes `stock_data` topic):
     - `cd python && python strategy.py`

7. Produce sample stock bars to Kafka (test producer)

   - The `KafkaDataFeed` in [python/strategy.py](python/strategy.py) expects Kafka messages to be a JSON array of records where each record for a bar has `T=='b'` and fields `o,h,l,c,v,t` (ISO UTC timestamp). Write a small test producer using `confluent_kafka.Producer` to push one or more records to topic `stock_data`.

8. Frontend

   - The `web/` folder contains the Vite React app. Typical local steps:
     - `cd web && npm install && npm run dev`

**RULES**
- Store ALL logs in /logs
- Don't use emojis in any of your output