-- Seed a dev user for local curl testing.
INSERT INTO users (id, email) VALUES
  ('00000000-0000-0000-0000-000000000001', 'dev@signal-flow.local')
ON CONFLICT (id) DO NOTHING;
