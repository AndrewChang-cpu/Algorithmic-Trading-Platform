#!/usr/bin/env python
# -*- coding: utf-8; py-indent-offset:4 -*-
from __future__ import (absolute_import, division, print_function,
                        unicode_literals)

import argparse
from datetime import datetime

# Import Backtrader and necessary utilities
import backtrader as bt
from backtrader.utils import flushfile  # win32 quick stdout flushing

# Import Confluent Kafka for Kafka consumer
from confluent_kafka import Producer, Consumer, KafkaError, TopicPartition
import json

###############################################################################
# Custom Kafka Data Feed Class
###############################################################################

class KafkaDataFeed(bt.feeds.DataBase):
    """ Custom Backtrader data feed that consumes stock data from a Kafka topic """

    params = (
        ('topic', 'stock_data'),
        ('consumer_group', 'backtrader-group'),
        ('kafka_servers', 'localhost:9092'),
        ('timeout', 1.0),  # Timeout for polling Kafka
        ('stocks', ['FAKEPACA']),  # List of stock symbols in the portfolio
        ('total_partitions', 1)  # Number of partitions for the topic
    )
    
    def __init__(self):
        super(KafkaDataFeed, self).__init__()
        
        if not self.p.topic:
            raise ValueError("Kafka topic must be specified")
        
        print('Initializing Kafka Data Feed...')
        
        # Initialize Kafka Consumer with a consumer group ID
        self.consumer = Consumer({
            'bootstrap.servers': self.p.kafka_servers,
            'group.id': self.p.consumer_group,
            'auto.offset.reset': 'earliest'  # Start consuming from the earliest message
        })

        # Manually assign partitions based on stocks in the portfolio
        self.assign_partitions_based_on_stocks()

        # Buffer to hold incoming data
        self.buffer = []

        # Indicate that this is a live data feed
        self.live_data = True

    def assign_partitions_based_on_stocks(self):
        """ Manually assign partitions based on the stocks in the portfolio """
        partitions = []
        for stock in self.p.stocks:
            # Calculate the partition based on the stock symbol
            print('Assigning partition based on stock:', stock)
            partition = 0  # CHANGE THIS LATER
            partitions.append(TopicPartition(self.p.topic, partition))

        # Assign the partitions to the consumer
        self.consumer.assign(partitions)

    def _load(self):
        """Load data from Kafka and format it for Backtrader"""
        
        # If buffer has data, push it to Backtrader
        if self.buffer:
            data = self.buffer.pop(0)
            print('DATA FOUND:', data)
            # Convert ISO timestamp to datetime and set to Backtrader's datetime format
            self.lines.datetime[0] = bt.date2num(datetime.strptime(data['t'], "%Y-%m-%dT%H:%M:%SZ"))
            self.lines.open[0] = float(data['o'])  # Open price
            self.lines.high[0] = float(data['h'])  # High price
            self.lines.low[0] = float(data['l'])   # Low price
            self.lines.close[0] = float(data['c']) # Close price
            self.lines.volume[0] = float(data['v']) # Volume
            self.lines.openinterest[0] = 0  # No open interest data
            return True
        
        # Poll Kafka for new messages
        msg = self.consumer.poll(self.p.timeout)
        if msg is None:
            return None  # No message received within timeout
        if msg.error():
            if msg.error().code() == KafkaError._PARTITION_EOF:
                return None  # End of partition event
            else:
                print(f"Kafka error: {msg.error()}")
                return False
        
        # Parse the Kafka message
        try:
            # Assuming the message is a JSON array of objects
            records = json.loads(msg.value().decode('utf-8'))
            for record in records:
                # Ensure the required fields are present
                if 'T' in record and record['T'] == 'b':  # Only process 'bar' type messages
                    required_fields = ['o', 'h', 'l', 'c', 'v', 't']
                    if all(field in record for field in required_fields):
                        # Add the data to the buffer for processing
                        self.buffer.append(record)

            return self._load()  # Recursive call to process the buffered data
        except json.JSONDecodeError as e:
            print(f"JSON decode error: {e}")
            return False

    def islive(self):
        """Indicate that this is a live data feed"""
        return True

###############################################################################
# Backtrader Strategy
###############################################################################

class TestStrategy(bt.Strategy):
    params = dict(
        stake=10,
    )

    def __init__(self):
        print('CALLED __init__')
        self.order = None
        # self.sma = bt.indicators.SimpleMovingAverage(self.data.close, period=self.p.smaperiod)
        self.datastatus = 1

        # Initialize Kafka producer
        self.kafka_producer = Producer({
            'bootstrap.servers': 'localhost:9092'  # Kafka broker address
        })

    def notify_data(self, data, status, *args, **kwargs):
        print('CALLED notify_data')
        if status == data.LIVE:
            self.datastatus = 1

    def notify_order(self, order):
        print('CALLED notify_order')
        if order.status in [order.Completed, order.Canceled, order.Rejected]:
            self.order = None  # Reset order

    def next(self):
        print('CALLED next')

        # Portfolio metrics
        portfolio_value = self.broker.getvalue()
        cash = self.broker.getcash()
        position = self.broker.getposition(self.data)

        print(f"Current Value: {portfolio_value}")
        print(f"Current Cash: {cash}")
        print(f"Position: {position}")

        # Create a message to send to Kafka
        message = {
            'portfolio_value': portfolio_value,
            'cash': cash,
            'position_size': position.size,
            'position_price': position.price,
            'datetime': self.data.datetime.datetime(0).strftime("%Y-%m-%dT%H:%M:%S"),
        }

        # Send message to Kafka topic 'portfolio_data'
        self.kafka_producer.produce(
            topic='portfolio_data',
            value=json.dumps(message)
        )
        self.kafka_producer.flush()  # Ensure the message is sent

        if not self.datastatus:
            return  # Wait until data is live

        if not self.position:  # No position
            self.order = self.buy(size=self.p.stake)
        else:  # Already in position
            self.order = self.sell(size=self.p.stake)

    def stop(self):
        print('CALLED stop')
        
        # Show final portfolio metrics
        print(f"Ending Value: {self.broker.getvalue()}")
        print(f"Final Cash: {self.broker.getcash()}")
        print(f"Total Trades: {self.broker.getposition(self.data)}")

###############################################################################
# Run Strategy Function
###############################################################################

def runstrategy():
    # Configuration values
    kafka_topic = 'stock_data'
    kafka_group = 'backtrader-group'
    kafka_server = 'localhost:9092'
    smaperiod = 5
    stake = 10
    initial_cash = 100000.0
    commission = 0
    plot_results = True

    # Create a cerebro instance
    cerebro = bt.Cerebro()

    # Add the custom Kafka data feed (or any other feed)
    print('Consumer Group:', kafka_group)
    kafka_feed = KafkaDataFeed(
        topic=kafka_topic,
        consumer_group=kafka_group,
        kafka_servers=kafka_server
    )
    cerebro.adddata(kafka_feed)

    # Add the strategy
    cerebro.addstrategy(TestStrategy, stake=stake)

    # Set initial cash (for paper trading)
    cerebro.broker.setcash(initial_cash)

    # Set commission (optional)
    cerebro.broker.setcommission(commission)  # Set a custom commission rate if needed

    # Run the strategy
    cerebro.run()

    # Show final portfolio value
    print(f'Final Portfolio Value: {cerebro.broker.getvalue()}')

    # Optionally plot the results
    if plot_results:
        cerebro.plot()

###############################################################################
# Main Execution
###############################################################################

if __name__ == '__main__':
    runstrategy()
