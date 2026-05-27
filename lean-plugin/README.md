# lean-atp Docker Image

## Build

```bash
docker build -t lean-atp:latest lean-plugin/
```

## What this image contains

- QuantConnect LEAN engine (base: `quantconnect/lean:latest`)
- `KafkaDataQueueHandler` plugin for real-time bar consumption from Kafka
- Auxiliary data files: symbol-properties-database.csv, map_files/, factor_files/

## Usage

Backtest mode (Celery binds job directory to /lean):
```bash
docker run --rm -v /tmp/atp-jobs/JOB_ID:/lean lean-atp:latest
```

Live mode (needs host network access for Kafka):
```bash
docker run -d --rm -v /tmp/atp-jobs/JOB_ID:/lean --add-host=host.docker.internal:host-gateway lean-atp:latest
```

The job directory must contain:
- `config.json` — LEAN configuration
- `algorithm/main.py` — the strategy file
- `data/` — materialized LEAN CSV files (written by Celery's data_materializer.py)
