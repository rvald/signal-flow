#!/bin/bash
set -e

echo "Starting database migrations..."
for file in /migrations/*.up.sql; do
    echo "Applying migration: $file"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$file"
done
echo "Migrations applied successfully!"
