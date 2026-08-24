-- ============================================================================
-- Indo Thai Order Position Engine - Database Schema & Analytical Queries
-- Compatible with PostgreSQL, SQLite, MySQL, and DuckDB
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. DATABASE SCHEMA DEFINITION (DDL)
-- ----------------------------------------------------------------------------

DROP TABLE IF EXISTS order_updates;

CREATE TABLE order_updates (
    event_id         VARCHAR(64)  NOT NULL,
    symbol           VARCHAR(32)  NOT NULL,
    transaction_type VARCHAR(4)   NOT NULL,
    quantity         INTEGER      NOT NULL,
    created_at       TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,

    -- Table Constraints
    CONSTRAINT pk_order_updates PRIMARY KEY (event_id),
    CONSTRAINT chk_transaction_type CHECK (transaction_type IN ('BUY', 'SELL')),
    CONSTRAINT chk_positive_quantity CHECK (quantity > 0)
);

-- Index for high-performance symbol aggregation queries
CREATE INDEX idx_order_updates_symbol ON order_updates (symbol);

-- ----------------------------------------------------------------------------
-- 2. BULK DATA IMPORT COMMANDS (Choose based on your database engine)
-- ----------------------------------------------------------------------------

-- PostgreSQL:
-- \copy order_updates(event_id, symbol, transaction_type, quantity) FROM 'order_updates (1).csv' WITH (FORMAT csv, HEADER true);

-- SQLite:
-- .mode csv
-- .import "order_updates (1).csv" order_updates

-- DuckDB:
-- INSERT INTO order_updates(event_id, symbol, transaction_type, quantity)
-- SELECT event_id, symbol, transaction_type, quantity FROM read_csv_auto('order_updates (1).csv');

-- ----------------------------------------------------------------------------
-- 3. ANALYTICAL QUERIES
-- ----------------------------------------------------------------------------

-- Query A: Standard Net Position Calculation per Symbol
-- Calculates BUY (+) and SELL (-) positions for all traded symbols.
SELECT 
    symbol,
    SUM(
        CASE 
            WHEN transaction_type = 'BUY'  THEN quantity
            WHEN transaction_type = 'SELL' THEN -quantity
            ELSE 0
        END
    ) AS net_position,
    COUNT(*) AS total_events,
    SUM(CASE WHEN transaction_type = 'BUY'  THEN quantity ELSE 0 END) AS total_buy_qty,
    SUM(CASE WHEN transaction_type = 'SELL' THEN quantity ELSE 0 END) AS total_sell_qty
FROM order_updates
GROUP BY symbol
ORDER BY symbol ASC;


-- Query B: Idempotent Net Position Calculation (Deduplicated by event_id)
-- Guarantees that only the first valid event_id wins in case of duplicates.
WITH deduplicated_orders AS (
    SELECT 
        event_id,
        symbol,
        transaction_type,
        quantity,
        ROW_NUMBER() OVER (
            PARTITION BY event_id 
            ORDER BY created_at ASC
        ) AS row_num
    FROM order_updates
)
SELECT 
    symbol,
    SUM(
        CASE 
            WHEN transaction_type = 'BUY'  THEN quantity
            WHEN transaction_type = 'SELL' THEN -quantity
            ELSE 0
        END
    ) AS net_position
FROM deduplicated_orders
WHERE row_num = 1
GROUP BY symbol
ORDER BY symbol ASC;


-- Query C: Position Summary View (Reusable SQL View)
CREATE VIEW view_symbol_net_positions AS
SELECT 
    symbol,
    SUM(
        CASE 
            WHEN transaction_type = 'BUY'  THEN quantity
            WHEN transaction_type = 'SELL' THEN -quantity
            ELSE 0
        END
    ) AS net_position
FROM order_updates
GROUP BY symbol;
