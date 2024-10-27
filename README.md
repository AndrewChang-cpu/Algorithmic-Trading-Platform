# Algorithmic-Trading-Platform
## Basic Workflow
- Start Kafka broker
- Run strategy ```python .\strategy.py --kafka_topic stock_data --kafka_group backtrader-group2 --kafka_server localhost:9092 --plot```

## Useful Commands
#### Test consumer:
kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic stock_data --from-beginning

#### Delete topic:
kafka-topics.sh --bootstrap-server localhost:9092 --delete --topic stock_data

#### Create topic:
kafka-topics.sh --bootstrap-server localhost:9092 --create --topic stock_data --partitions 1 --replication-factor 1

#### List topics:
kafka-topics.sh --bootstrap-server localhost:9092 --list

#### List consumer groups:
kafka-consumer-groups.sh --bootstrap-server localhost:9092 --list