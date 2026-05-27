CREATE TABLE market_data (
  time        TIMESTAMPTZ NOT NULL,
  symbol      VARCHAR(20) NOT NULL,
  resolution  VARCHAR(10) NOT NULL,
  open        DECIMAL(20,4),
  high        DECIMAL(20,4),
  low         DECIMAL(20,4),
  close       DECIMAL(20,4),
  volume      BIGINT
);
SELECT create_hypertable('market_data', 'time');
CREATE UNIQUE INDEX ON market_data (symbol, resolution, time DESC);
