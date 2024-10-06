from confluent_kafka import Consumer, TopicPartition, KafkaError
import json

def test_consumer_with_group(topic, group_id, kafka_server, stocks, total_partitions):
    # Kafka consumer configuration with a consumer group
    consumer_conf = {
        'bootstrap.servers': kafka_server,
        'group.id': group_id,
        'auto.offset.reset': 'earliest',  # Start from the earliest message
    }

    # Create Kafka Consumer
    consumer = Consumer(consumer_conf)

    # Function to assign partitions based on stock symbols
    def assign_partitions_based_on_stocks():
        """ Manually assign partitions based on the stocks in the portfolio """
        partitions = []
        for stock in stocks:
            # Calculate the partition based on the stock symbol
            partition = 0 # CHANGE THIS LATER
            partitions.append(TopicPartition(topic, partition))

        # Assign the partitions to the consumer
        consumer.assign(partitions)
        print(f"Assigned partitions: {partitions} based on stocks: {stocks}")

    # Assign partitions based on the provided stocks
    assign_partitions_based_on_stocks()

    print(f"Subscribed to topic '{topic}' with consumer group '{group_id}' and waiting for messages...\n")

    try:
        while True:
            # Poll for new messages
            msg = consumer.poll(1.0)  # Poll timeout of 1 second

            if msg is None:
                continue  # No message received in the last second
            if msg.error():
                if msg.error().code() == KafkaError._PARTITION_EOF:
                    print(f"Reached end of partition for {msg.topic()} [{msg.partition()}]")
                else:
                    print(f"Kafka error: {msg.error()}")
                    break
            else:
                # Successfully received a message, print it
                print(f"Received message from partition {msg.partition()}: {msg.value().decode('utf-8')}")
    except KeyboardInterrupt:
        print("Consumer interrupted. Closing...")
    finally:
        # Close the consumer to release resources
        consumer.close()

if __name__ == "__main__":
    # Test parameters
    topic = 'stock_data'
    group_id = 'backtrader-group'
    kafka_server = 'localhost:9092'
    stocks = ['FAKEPACA']  # Stocks in the portfolio
    total_partitions = 1  # Number of partitions in the topic

    # Test the consumer with the group logic
    test_consumer_with_group(topic, group_id, kafka_server, stocks, total_partitions)
