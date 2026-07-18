#!/bin/sh
# Mounted at /docker-entrypoint-initdb.d - the postgres image runs every .sh
# script here (as $POSTGRES_USER, the superuser) on first init. The actual
# SQL lives in ./migrations, mounted separately at /migrations-src so the
# entrypoint's own *.sql auto-runner doesn't also pick up 0003_roles.sql and
# execute it without the -v substitution it needs.
set -eu

for f in /migrations-src/*.sql; do
  echo "applying $f"
  if [ "$(basename "$f")" = "0003_roles.sql" ]; then
    psql -v ON_ERROR_STOP=1 \
      -v api_password="${API_DB_PASSWORD}" \
      -v consumer_password="${CONSUMER_DB_PASSWORD}" \
      --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
      -f "$f"
  else
    psql -v ON_ERROR_STOP=1 \
      --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
      -f "$f"
  fi
done
