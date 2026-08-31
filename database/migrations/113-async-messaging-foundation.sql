-- 113-async-messaging-foundation.sql
-- PR1: Reliable Messaging Foundation
-- Adds outbox_events and consumer_inbox tables for transactional outbox pattern.
-- No business schema changes. No provider_execution, credit_ledger, or payment changes.

BEGIN;

-- ============================================================================
-- Outbox events: transactional outbox for reliable message publishing
-- ============================================================================

CREATE TABLE IF NOT EXISTS outbox_events (
    id              BIGSERIAL       PRIMARY KEY,
    event_id        TEXT            NOT NULL,
    aggregate_type  TEXT            NOT NULL,
    aggregate_id    TEXT            NOT NULL,
    event_type      TEXT            NOT NULL,
    event_version   INTEGER         NOT NULL DEFAULT 1,
    payload         JSONB           NOT NULL,
    trace_id        TEXT,
    status          TEXT            NOT NULL DEFAULT 'pending',
    attempt_count   INTEGER         NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ     NOT NULL DEFAULT now(),
    published_at    TIMESTAMPTZ,
    claimed_at      TIMESTAMPTZ,
    claim_owner     TEXT,
    last_error      TEXT,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_outbox_events_event_id
    ON outbox_events (event_id);

CREATE INDEX IF NOT EXISTS idx_outbox_status_next_attempt
    ON outbox_events (status, next_attempt_at);

CREATE INDEX IF NOT EXISTS idx_outbox_aggregate
    ON outbox_events (aggregate_type, aggregate_id);

CREATE INDEX IF NOT EXISTS idx_outbox_created_at
    ON outbox_events (created_at);

CREATE INDEX IF NOT EXISTS idx_outbox_published
    ON outbox_events (published_at) WHERE status = 'published';

-- ============================================================================
-- Consumer inbox: deduplication for message consumers
-- ============================================================================

CREATE TABLE IF NOT EXISTS consumer_inbox (
    id              BIGSERIAL       PRIMARY KEY,
    consumer_name   TEXT            NOT NULL,
    event_id        TEXT            NOT NULL,
    processed_at    TIMESTAMPTZ,
    result          TEXT,
    metadata        JSONB,
    error_message   TEXT,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_consumer_inbox_consumer_event
    ON consumer_inbox (consumer_name, event_id);

CREATE INDEX IF NOT EXISTS idx_inbox_consumer_event
    ON consumer_inbox (consumer_name, event_id);

CREATE INDEX IF NOT EXISTS idx_inbox_processed_at
    ON consumer_inbox (processed_at);

-- Status values documented as comments (no ENUM to allow extensibility):
--   pending    — transaction committed, waiting for OutboxPublisher
--   publishing — claimed by an OutboxPublisher instance
--   published  — successfully published to RabbitMQ and confirmed
--   failed     — exceeded max retry attempts

COMMIT;
