CREATE EXTENSION IF NOT EXISTS "vector";

CREATE TABLE signals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    source_url    TEXT NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    content       TEXT DEFAULT '',
    distillation  TEXT DEFAULT '',
    metadata      JSONB DEFAULT '{}',
    scope         TEXT NOT NULL DEFAULT 'private' CHECK (scope IN ('private', 'team')),
    vector        vector(1536),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_url, tenant_id)
);

CREATE INDEX idx_signals_tenant_id ON signals (tenant_id);

-- Row-Level Security
ALTER TABLE signals ENABLE ROW LEVEL SECURITY;
ALTER TABLE signals FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON signals
    USING (tenant_id = current_setting('app.current_tenant_id')::uuid);

CREATE POLICY tenant_insert ON signals
    FOR INSERT WITH CHECK (tenant_id = current_setting('app.current_tenant_id')::uuid);
