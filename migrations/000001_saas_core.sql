CREATE TABLE IF NOT EXISTS tenants (
    id uuid PRIMARY KEY,
    name varchar(255) NOT NULL,
    slug varchar(255) NOT NULL UNIQUE,
    api_key_prefix varchar(32) NOT NULL UNIQUE,
    api_key_hash varchar(255) NOT NULL,
    ai_enabled boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email varchar(255) NOT NULL,
    password_hash varchar(255) NOT NULL,
    name varchar(255) NOT NULL,
    role varchar(50) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_tenant_email ON users (tenant_id, email);

CREATE TABLE IF NOT EXISTS instances (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name varchar(255) NOT NULL,
    status varchar(50) NOT NULL DEFAULT 'created',
    engine_instance_id varchar(255),
    webhook_url varchar(500),
    ai_enabled boolean NOT NULL DEFAULT false,
    ai_auto_reply boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS runtime_session_states (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    instance_id uuid NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    status varchar(50) NOT NULL DEFAULT 'created',
    last_seen_status varchar(50) NOT NULL DEFAULT 'created',
    last_event_type varchar(100) NOT NULL DEFAULT 'created',
    last_event_source varchar(50) NOT NULL DEFAULT 'system',
    connected boolean NOT NULL DEFAULT false,
    logged_in boolean NOT NULL DEFAULT false,
    pairing_active boolean NOT NULL DEFAULT false,
    disconnect_reason varchar(255),
    last_error text,
    last_event_at timestamptz NULL,
    last_seen_at timestamptz NULL,
    last_connected_at timestamptz NULL,
    last_disconnected_at timestamptz NULL,
    last_paired_at timestamptz NULL,
    last_logout_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_session_state_instance ON runtime_session_states (tenant_id, instance_id);
CREATE INDEX IF NOT EXISTS idx_runtime_session_state_lookup ON runtime_session_states (tenant_id, instance_id, updated_at);

CREATE TABLE IF NOT EXISTS runtime_session_events (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    instance_id uuid NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    event_type varchar(100) NOT NULL,
    event_source varchar(50) NOT NULL,
    status varchar(50) NOT NULL,
    connected boolean NOT NULL DEFAULT false,
    logged_in boolean NOT NULL DEFAULT false,
    pairing_active boolean NOT NULL DEFAULT false,
    disconnect_reason varchar(255),
    error_message text,
    message text,
    payload text,
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runtime_session_events_lookup ON runtime_session_events (tenant_id, instance_id, event_type, occurred_at);

CREATE TABLE IF NOT EXISTS contacts (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    phone varchar(50) NOT NULL,
    name varchar(255) NOT NULL,
    email varchar(255),
    instance_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_contacts_tenant_phone ON contacts (tenant_id, phone);

CREATE TABLE IF NOT EXISTS tags (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name varchar(255) NOT NULL,
    color varchar(32),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_tenant_name ON tags (tenant_id, lower(name));

CREATE TABLE IF NOT EXISTS contact_tags (
    contact_id uuid NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (contact_id, tag_id)
);

CREATE TABLE IF NOT EXISTS notes (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS broadcast_jobs (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    instance_id uuid NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    status varchar(50) NOT NULL,
    message text NOT NULL,
    rate_per_hour integer NOT NULL DEFAULT 0,
    delay_sec integer NOT NULL DEFAULT 0,
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    worker_id varchar(100),
    last_error text,
    scheduled_at timestamptz NULL,
    available_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz NULL,
    completed_at timestamptz NULL,
    failed_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_broadcast_jobs_ready ON broadcast_jobs (status, available_at, created_at);

CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name varchar(255) NOT NULL,
    url varchar(500) NOT NULL,
    inbound_enabled boolean NOT NULL DEFAULT true,
    outbound_enabled boolean NOT NULL DEFAULT true,
    signing_secret varchar(255),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    endpoint_id uuid NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    direction varchar(50) NOT NULL,
    event_type varchar(100) NOT NULL,
    status varchar(50) NOT NULL,
    response_status integer NOT NULL DEFAULT 0,
    request_body text NOT NULL,
    response_body text,
    error_message text,
    delivered_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_tenant_direction ON webhook_deliveries (tenant_id, direction, event_type, created_at);

CREATE TABLE IF NOT EXISTS ai_settings (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT false,
    auto_reply boolean NOT NULL DEFAULT false,
    provider varchar(100) NOT NULL DEFAULT 'openai',
    model varchar(100) NOT NULL,
    base_url varchar(500),
    system_prompt text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ai_conversation_messages (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    instance_id uuid NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    conversation_key varchar(255) NOT NULL,
    role varchar(50) NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_memory_lookup ON ai_conversation_messages (tenant_id, instance_id, conversation_key, created_at);
