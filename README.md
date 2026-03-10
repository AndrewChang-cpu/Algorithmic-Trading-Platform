# Algorithmic-Trading-Platform
## Basic Workflow
- ```export NAME=k8s.andrw.tech```
- ```export KOPS_STATE_STORE=s3://kops-config-algo-trading```
- ```kops create -f kops.yaml```: This provisions the k8s cluster, installs ArgoCD, and applies the CRDs

## Local Workflow (not tested recently)
- Start Kafka broker using Docker Compose: ```docker-compose -f local-kafka-docker-compose.yml up```
- Run Redis (eventually replace with Kafka): ```docker run -d -p 6379:6379 redis```
- Run Celery worker: ```celery -A celery_worker worker --loglevel=info``` (single-threaded for debugging ```celery -A celery_worker worker --loglevel=info -P solo```)

- Run strategy ```python .\strategy.py --kafka_topic stock_data --kafka_group backtrader-group2 --kafka_server localhost:9092 --plot``` (outdated)

## Useful Commands
#### Test consumer:
docker exec -it [CONTAINER ID] /bin/sh   
kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic stock_data --from-beginning

#### Delete topic:
kafka-topics.sh --bootstrap-server localhost:9092 --delete --topic stock_data

#### Create topic:
kafka-topics.sh --bootstrap-server localhost:9092 --create --topic stock_data --partitions 1 --replication-factor 1

#### List topics:
kafka-topics.sh --bootstrap-server localhost:9092 --list

#### List consumer groups:
kafka-consumer-groups.sh --bootstrap-server localhost:9092 --list

## Setup
kOps addons to be added to S3 state store:
- External Secrets Operator (ESO) Helm chart
- Argo CD Helm chart
- Argo CD root-app.yaml