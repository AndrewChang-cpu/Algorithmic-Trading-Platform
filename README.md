# Algorithmic-Trading-Platform
## Basic Workflow
- Start Kafka broker using Docker Compose: ```docker-compose -f local-kafka-docker-compose.yml up```
- Run Redis (eventually replace with Kafka): ```docker run -d -p 6379:6379 redis```
- Run Celery worker: ```celery -A celery_worker worker --loglevel=info```

- Run strategy ```python .\strategy.py --kafka_topic stock_data --kafka_group backtrader-group2 --kafka_server localhost:9092 --plot``` (outdated)

## Useful Commands
#### Test consumer:
docker exec -it 33e050408928 /bin/sh   
kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic stock_data --from-beginning

#### Delete topic:
kafka-topics.sh --bootstrap-server localhost:9092 --delete --topic stock_data

#### Create topic:
kafka-topics.sh --bootstrap-server localhost:9092 --create --topic stock_data --partitions 1 --replication-factor 1

#### List topics:
kafka-topics.sh --bootstrap-server localhost:9092 --list

#### List consumer groups:
kafka-consumer-groups.sh --bootstrap-server localhost:9092 --list