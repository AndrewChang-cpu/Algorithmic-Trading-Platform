FROM python:3.10-slim

WORKDIR /app
COPY requirements.txt /app/
RUN pip install --no-cache-dir -r requirements.txt
COPY . /app/

# Set the environment variable for Celery
ENV CELERY_BROKER_URL=redis://redis-service:6379/0

# Command to run the Celery worker
CMD ["celery", "-A", "celery_worker", "worker", "--loglevel=info"]