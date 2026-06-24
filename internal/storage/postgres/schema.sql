-- H2O GPS Platform — PostgreSQL + TimescaleDB Schema
-- Run this migration once on a fresh database with TimescaleDB extension enabled

-- ═══════════════════════════════════════════════════════════════
-- EXTENSIONS
-- ═══════════════════════════════════════════════════════════════

CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS postgis;  -- for geospatial queries

-- ═══════════════════════════════════════════════════════════════
-- TENANTS
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(100) UNIQUE NOT NULL,
    plan        VARCHAR(50)  NOT NULL DEFAULT 'starter', -- starter, professional, enterprise
    max_devices INTEGER      NOT NULL DEFAULT 50,
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenants_slug ON tenants(slug);

-- ═══════════════════════════════════════════════════════════════
-- USERS
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email           VARCHAR(320) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    role            VARCHAR(50)  NOT NULL DEFAULT 'viewer', -- admin, manager, viewer
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_email  ON users(email);

-- ═══════════════════════════════════════════════════════════════
-- VEHICLES
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE vehicles (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id     UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    reg_number    VARCHAR(50)  NOT NULL,           -- e.g., KA-01-AB-1234
    make          VARCHAR(100),
    model         VARCHAR(100),
    year          SMALLINT,
    fuel_type     VARCHAR(30),                     -- petrol, diesel, electric, cng
    color         VARCHAR(50),
    icon          VARCHAR(100) DEFAULT 'car',
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    metadata      JSONB        NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE(tenant_id, reg_number)
);

CREATE INDEX idx_vehicles_tenant ON vehicles(tenant_id);

-- ═══════════════════════════════════════════════════════════════
-- DEVICES
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE devices (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id     UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    vehicle_id    UUID         REFERENCES vehicles(id) ON DELETE SET NULL,
    imei          VARCHAR(20)  NOT NULL UNIQUE,
    serial        VARCHAR(50),
    model         VARCHAR(100),                    -- e.g., GT06N, GT06E
    sim_number    VARCHAR(20),
    sim_operator  VARCHAR(50)  DEFAULT 'airtel',
    firmware      VARCHAR(50),
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    installed_at  TIMESTAMPTZ,
    last_seen_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_devices_tenant ON devices(tenant_id);
CREATE INDEX idx_devices_imei   ON devices(imei);
CREATE INDEX idx_devices_vehicle ON devices(vehicle_id);

-- ═══════════════════════════════════════════════════════════════
-- LOCATIONS (TimescaleDB Hypertable)
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE locations (
    id            BIGSERIAL,
    tenant_id     UUID        NOT NULL,
    device_id     UUID        NOT NULL,
    imei          VARCHAR(20) NOT NULL,
    latitude      DOUBLE PRECISION NOT NULL,
    longitude     DOUBLE PRECISION NOT NULL,
    speed         REAL        NOT NULL DEFAULT 0,     -- km/h
    heading       REAL        NOT NULL DEFAULT 0,     -- degrees 0-360
    satellites    SMALLINT    NOT NULL DEFAULT 0,
    gps_fixed     BOOLEAN     NOT NULL DEFAULT FALSE,
    ignition      BOOLEAN     NOT NULL DEFAULT FALSE,
    altitude      REAL,
    odometer      REAL,                               -- cumulative km
    mcc           SMALLINT,
    mnc           SMALLINT,
    lac           INTEGER,
    cell_id       BIGINT,
    packet_serial SMALLINT,
    raw_data      BYTEA,                              -- original binary for audit
    recorded_at   TIMESTAMPTZ NOT NULL,               -- device timestamp
    received_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(), -- server receive time

    -- TimescaleDB requires the time column in the primary key
    PRIMARY KEY (id, recorded_at)
);

-- Convert to TimescaleDB hypertable, partitioned by time (1 week chunks)
SELECT create_hypertable('locations', 'recorded_at',
    chunk_time_interval => INTERVAL '1 week',
    if_not_exists => TRUE
);

-- Compression (automatically compress chunks older than 7 days)
ALTER TABLE locations SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id',
    timescaledb.compress_orderby   = 'recorded_at DESC'
);

SELECT add_compression_policy('locations', INTERVAL '7 days');

-- Retention: keep raw data for 1 year, then drop
SELECT add_retention_policy('locations', INTERVAL '1 year');

-- Indexes for common query patterns
CREATE INDEX idx_locations_device_time ON locations(device_id, recorded_at DESC);
CREATE INDEX idx_locations_tenant_time ON locations(tenant_id, recorded_at DESC);
CREATE INDEX idx_locations_imei_time   ON locations(imei, recorded_at DESC);
-- BRIN index for time-range scans (very efficient on large time-series)
CREATE INDEX idx_locations_recorded_brin ON locations USING BRIN(recorded_at);

-- ═══════════════════════════════════════════════════════════════
-- TRIPS
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE trips (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID        NOT NULL REFERENCES tenants(id),
    device_id       UUID        NOT NULL REFERENCES devices(id),
    vehicle_id      UUID        REFERENCES vehicles(id),
    start_lat       DOUBLE PRECISION,
    start_lng       DOUBLE PRECISION,
    end_lat         DOUBLE PRECISION,
    end_lng         DOUBLE PRECISION,
    start_address   TEXT,
    end_address     TEXT,
    distance_km     REAL        NOT NULL DEFAULT 0,
    max_speed       REAL        NOT NULL DEFAULT 0,
    avg_speed       REAL        NOT NULL DEFAULT 0,
    duration_secs   INTEGER     NOT NULL DEFAULT 0,
    idle_secs       INTEGER     NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_trips_device ON trips(device_id, started_at DESC);
CREATE INDEX idx_trips_tenant ON trips(tenant_id, started_at DESC);

-- ═══════════════════════════════════════════════════════════════
-- GEOFENCES
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE geofences (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    type        VARCHAR(30)  NOT NULL DEFAULT 'polygon', -- circle, polygon, rectangle
    color       VARCHAR(20)  DEFAULT '#3B82F6',
    coordinates JSONB        NOT NULL,                   -- GeoJSON polygon/circle
    center_lat  DOUBLE PRECISION,
    center_lng  DOUBLE PRECISION,
    radius_m    REAL,                                    -- for circle type
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_geofences_tenant ON geofences(tenant_id);

-- ═══════════════════════════════════════════════════════════════
-- ALERTS
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE alert_rules (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    type        VARCHAR(50)  NOT NULL, -- overspeed, geofence_in, geofence_out, idle, sos, power_cut
    device_ids  UUID[]       NOT NULL DEFAULT '{}', -- empty = all devices
    config      JSONB        NOT NULL DEFAULT '{}', -- type-specific config (e.g., {speed_kmh: 100})
    channels    JSONB        NOT NULL DEFAULT '{}', -- {email: true, push: true, sms: false}
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE alerts (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID         NOT NULL REFERENCES tenants(id),
    device_id   UUID         NOT NULL REFERENCES devices(id),
    rule_id     UUID         REFERENCES alert_rules(id),
    type        VARCHAR(50)  NOT NULL,
    severity    VARCHAR(20)  NOT NULL DEFAULT 'info', -- info, warning, critical
    latitude    DOUBLE PRECISION,
    longitude   DOUBLE PRECISION,
    speed       REAL,
    message     TEXT,
    is_read     BOOLEAN      NOT NULL DEFAULT FALSE,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alerts_tenant_time ON alerts(tenant_id, triggered_at DESC);
CREATE INDEX idx_alerts_device ON alerts(device_id, triggered_at DESC);

-- ═══════════════════════════════════════════════════════════════
-- REFRESH TOKENS
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  VARCHAR(255) NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);

-- ═══════════════════════════════════════════════════════════════
-- ROW LEVEL SECURITY (Multi-tenant isolation)
-- ═══════════════════════════════════════════════════════════════

ALTER TABLE devices    ENABLE ROW LEVEL SECURITY;
ALTER TABLE vehicles   ENABLE ROW LEVEL SECURITY;
ALTER TABLE locations  ENABLE ROW LEVEL SECURITY;
ALTER TABLE trips      ENABLE ROW LEVEL SECURITY;
ALTER TABLE geofences  ENABLE ROW LEVEL SECURITY;
ALTER TABLE alerts     ENABLE ROW LEVEL SECURITY;
ALTER TABLE users      ENABLE ROW LEVEL SECURITY;

-- App role bypasses RLS (used by backend with set_config)
CREATE POLICY tenant_isolation ON devices
    USING (tenant_id = current_setting('app.current_tenant_id')::UUID);

CREATE POLICY tenant_isolation ON locations
    USING (tenant_id = current_setting('app.current_tenant_id')::UUID);

CREATE POLICY tenant_isolation ON vehicles
    USING (tenant_id = current_setting('app.current_tenant_id')::UUID);

-- ═══════════════════════════════════════════════════════════════
-- CONTINUOUS AGGREGATES (TimescaleDB — hourly stats per device)
-- ═══════════════════════════════════════════════════════════════

CREATE MATERIALIZED VIEW hourly_device_stats
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', recorded_at) AS bucket,
    device_id,
    tenant_id,
    COUNT(*)            AS point_count,
    MAX(speed)          AS max_speed,
    AVG(speed)          AS avg_speed,
    SUM(CASE WHEN ignition THEN 1 ELSE 0 END) AS ignition_on_count
FROM locations
GROUP BY bucket, device_id, tenant_id;

-- Refresh policy: update every 30 minutes
SELECT add_continuous_aggregate_policy('hourly_device_stats',
    start_offset => INTERVAL '3 hours',
    end_offset   => INTERVAL '1 hour',
    schedule_interval => INTERVAL '30 minutes'
);
