DROP INDEX IF EXISTS idx_signals_url_hash_tenant;
ALTER TABLE signals DROP COLUMN IF EXISTS url_hash;

ALTER TABLE user_credentials DROP COLUMN IF EXISTS needs_reauth;
ALTER TABLE user_credentials DROP COLUMN IF EXISTS last_seen_id;
