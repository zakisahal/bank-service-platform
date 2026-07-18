-- Least-privilege application roles. Run once by the RDS master user /
-- local superuser - the application itself never connects as this role and
-- never has permission to run this file. Expects two psql variables,
-- api_password and consumer_password, e.g.:
--   psql "$ADMIN_DATABASE_URL" -v api_password="$API_DB_PASSWORD" \
--        -v consumer_password="$CONSUMER_DB_PASSWORD" -f 0003_roles.sql
--
-- Two roles, not one, because the HTTP API and the event consumer are two
-- different trust boundaries with disjoint data needs - collapsing them
-- into one role would hand the API write access to the audit trail (which
-- it never touches) and the consumer read access to account balances
-- (which it never needs either).

-- Plain CREATE ROLE has no IF NOT EXISTS; \gexec runs the generated
-- statement only when the role is actually missing, so this file is safe
-- to re-run (e.g. on every `helm upgrade`).
SELECT 'CREATE ROLE bank_service_api LOGIN'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'bank_service_api')
\gexec

SELECT 'CREATE ROLE bank_service_consumer LOGIN'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'bank_service_consumer')
\gexec

ALTER ROLE bank_service_api WITH PASSWORD :'api_password';
ALTER ROLE bank_service_consumer WITH PASSWORD :'consumer_password';

-- --- bank_service_api: read-only on accounts, nothing else -------------
-- Deliberately cannot: INSERT/UPDATE/DELETE on accounts, touch
-- processed_events or audit_log at all, run any DDL, or see any other
-- role's data. This role only ever backs GET /api/accounts/{id}.
GRANT CONNECT ON DATABASE bank TO bank_service_api;
GRANT USAGE ON SCHEMA public TO bank_service_api;
GRANT SELECT ON accounts TO bank_service_api;

-- --- bank_service_consumer: append-only on the event tables -------------
-- Deliberately cannot: read or write accounts at all, DELETE or UPDATE its
-- own audit rows once written (an audit trail that could be edited by the
-- thing being audited isn't one), or run DDL. It can SELECT on
-- processed_events because the dedupe check is an INSERT ... ON CONFLICT,
-- which needs the table's own unique index but not a broader read grant on
-- audit_log.
GRANT CONNECT ON DATABASE bank TO bank_service_consumer;
GRANT USAGE ON SCHEMA public TO bank_service_consumer;
GRANT SELECT, INSERT ON processed_events TO bank_service_consumer;
GRANT INSERT ON audit_log TO bank_service_consumer;
GRANT USAGE ON SEQUENCE audit_log_id_seq TO bank_service_consumer;

-- Neither role is the table owner, neither is superuser, and the default
-- PUBLIC grant that Postgres applies to every new schema/object is revoked
-- so nothing is reachable by accident.
REVOKE ALL ON SCHEMA public FROM PUBLIC;
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES FROM PUBLIC;
