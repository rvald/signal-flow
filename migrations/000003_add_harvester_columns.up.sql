ALTER TABLE user_credentials
    ADD COLUMN last_seen_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN needs_reauth BOOLEAN NOT NULL DEFAULT false;

-- URL hash column + unique index for O(1) dedup lookups in the Harvester.
ALTER TABLE signals
    ADD COLUMN url_hash TEXT;

CREATE UNIQUE INDEX idx_signals_url_hash_tenant
    ON signals (url_hash, tenant_id);
