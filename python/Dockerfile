FROM python:3.11-slim

# Install Docker CLI (needed by lean_runner.py to call docker run/stop)
RUN apt-get update && apt-get install -y \
    docker.io \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY *.py .

RUN mkdir -p /logs

CMD ["celery", "-A", "celery_worker", "worker", "--loglevel=info", "--logfile=/logs/celery.log"]
