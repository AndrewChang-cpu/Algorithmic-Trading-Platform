using System;
using System.Collections;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Linq;
using System.Text.Json;
using System.Threading;
using Confluent.Kafka;
using QuantConnect;
using QuantConnect.Data;
using QuantConnect.Data.Market;
using QuantConnect.Interfaces;
using QuantConnect.Packets;

namespace ATP.Lean.Plugin
{
    public class KafkaDataQueueHandler : IDataQueueHandler
    {
        private IConsumer<string, string>? _consumer;
        private Thread? _consumeThread;
        private volatile bool _running;
        private string _bootstrapServers = "localhost:9092";
        private string _jobId = "default";

        // Per-symbol queue and handler, one entry per Subscribe call
        private readonly Dictionary<Symbol, ConcurrentQueue<BaseData>> _queues = new();
        private readonly Dictionary<Symbol, EventHandler> _handlers = new();

        public bool IsConnected => _consumer != null && _running;

        public void SetJob(LiveNodePacket job)
        {
            if (job.BrokerageData.TryGetValue("kafka-bootstrap-servers", out var bs))
                _bootstrapServers = bs;
            if (job.BrokerageData.TryGetValue("job-id", out var jid))
                _jobId = jid;
        }

        // GetNextTicks is the legacy pull path; returning empty is safe because
        // LEAN will use the per-symbol enumerators returned by Subscribe instead.
        public IEnumerable<BaseData> GetNextTicks() => Enumerable.Empty<BaseData>();

        public IEnumerator<BaseData> Subscribe(SubscriptionDataConfig config, EventHandler newDataAvailableHandler)
        {
            var queue = new ConcurrentQueue<BaseData>();
            _queues[config.Symbol] = queue;
            _handlers[config.Symbol] = newDataAvailableHandler;

            if (_consumer == null)
            {
                var kafkaConfig = new ConsumerConfig
                {
                    BootstrapServers = _bootstrapServers,
                    GroupId = $"lean-live-{_jobId}",
                    AutoOffsetReset = AutoOffsetReset.Latest,
                    EnableAutoCommit = true
                };
                _consumer = new ConsumerBuilder<string, string>(kafkaConfig).Build();
                _consumer.Subscribe("stock_data");
                _running = true;

                _consumeThread = new Thread(ConsumeLoop) { IsBackground = true, Name = "KafkaConsumer" };
                _consumeThread.Start();
            }

            return new SymbolEnumerator(queue, () => _running);
        }

        public void Unsubscribe(SubscriptionDataConfig config)
        {
            _queues.Remove(config.Symbol);
            _handlers.Remove(config.Symbol);

            if (_queues.Count == 0)
            {
                _running = false;
                _consumer?.Close();
                _consumer?.Dispose();
                _consumer = null;
            }
        }

        private void ConsumeLoop()
        {
            while (_running)
            {
                try
                {
                    var result = _consumer!.Consume(TimeSpan.FromMilliseconds(500));
                    if (result?.Message?.Value == null) continue;

                    var bar = ParseBar(result.Message.Value);
                    if (bar == null) continue;

                    if (_queues.TryGetValue(bar.Symbol, out var queue))
                    {
                        queue.Enqueue(bar);
                        _handlers[bar.Symbol].Invoke(this, EventArgs.Empty);
                    }
                }
                catch (OperationCanceledException) { break; }
                catch (Exception ex)
                {
                    Console.Error.WriteLine($"[KafkaDataQueueHandler] Consume error: {ex.Message}");
                }
            }
        }

        private TradeBar? ParseBar(string json)
        {
            try
            {
                using var doc = JsonDocument.Parse(json);
                var root = doc.RootElement;

                // Handle both direct object and array (Alpaca sends arrays)
                JsonElement barEl = root.ValueKind == JsonValueKind.Array
                    ? root.EnumerateArray().FirstOrDefault(e => e.GetProperty("T").GetString() == "b")
                    : root;

                if (barEl.ValueKind == JsonValueKind.Undefined) return null;

                var ticker = barEl.GetProperty("S").GetString()!;
                var symbol = Symbol.Create(ticker, SecurityType.Equity, Market.USA);

                var time = barEl.GetProperty("t").GetDateTime().ToUniversalTime();
                var open = barEl.GetProperty("o").GetDecimal();
                var high = barEl.GetProperty("h").GetDecimal();
                var low = barEl.GetProperty("l").GetDecimal();
                var close = barEl.GetProperty("c").GetDecimal();
                var volume = barEl.GetProperty("v").GetDecimal();

                return new TradeBar(time, symbol, open, high, low, close, (long)volume, TimeSpan.FromMinutes(1));
            }
            catch
            {
                return null;
            }
        }

        public void Dispose()
        {
            _running = false;
            _consumer?.Close();
            _consumer?.Dispose();
        }

        // Enumerator returned per-symbol. MoveNext dequeues the next bar (or yields
        // null if the queue is empty) and returns true while the feed is live.
        private sealed class SymbolEnumerator : IEnumerator<BaseData>
        {
            private readonly ConcurrentQueue<BaseData> _queue;
            private readonly Func<bool> _isRunning;
            private BaseData? _current;

            public SymbolEnumerator(ConcurrentQueue<BaseData> queue, Func<bool> isRunning)
            {
                _queue = queue;
                _isRunning = isRunning;
            }

            public BaseData Current => _current!;
            object IEnumerator.Current => _current!;

            public bool MoveNext()
            {
                _queue.TryDequeue(out _current); // null if empty; LEAN skips nulls
                return _isRunning() || _current != null;
            }

            public void Reset() { }
            public void Dispose() { }
        }
    }
}
