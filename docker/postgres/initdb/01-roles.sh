#!/usr/bin/env bash
# The gokick roles and database — the ONE definition, shared by the dev database
# (compose service db), the test cluster (db-test) and CI. The postgres image runs
# every script in /docker-entrypoint-initdb.d once, on an EMPTY data directory, as
# the superuser; the same script bootstraps any other cluster when run by hand:
#
#   POSTGRES_USER=postgres POSTGRES_DB=postgres PGHOST=… ./01-roles.sh
#
# Three login roles, because Row-Level Security is only as strong as the role the
# application connects as (see the Postgres adapter plan, section 6):
#
#   gokick_owner   owns the schema and runs the migrations (APP_DB_MIGRATE_URL);
#                  the application never uses it at runtime.
#   gokick_app     the tenant plane (APP_DB_URL): NOBYPASSRLS and owns nothing, so
#                  every row it touches passes the tenant policies.
#   gokick_system  the system plane (APP_DB_SYSTEM_URL): BYPASSRLS, for the
#                  cross-tenant work — platform, CLI, worker claims, login, audit.
#
# Grants and policies live in the migrations (migrations/postgres); roles are
# cluster objects, so they are created here and never by a migration.
#
# Idempotent: an existing role or database is left as it is, so re-running the
# script against a bootstrapped cluster changes nothing. The passwords default to
# the role names — fine for the dev and test containers, which publish no port.
# Anything reachable from elsewhere sets the GOKICK_*_PASSWORD variables.
set -euo pipefail

psql -v ON_ERROR_STOP=1 --no-psqlrc \
	--username "${POSTGRES_USER:-postgres}" --dbname "${POSTGRES_DB:-postgres}" \
	-v owner_pw="${GOKICK_OWNER_PASSWORD:-gokick_owner}" \
	-v app_pw="${GOKICK_APP_PASSWORD:-gokick_app}" \
	-v system_pw="${GOKICK_SYSTEM_PASSWORD:-gokick_system}" \
	<<'SQL'
SELECT format('CREATE ROLE gokick_owner LOGIN NOBYPASSRLS PASSWORD %L', :'owner_pw')
 WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'gokick_owner') \gexec
SELECT format('CREATE ROLE gokick_app LOGIN NOBYPASSRLS PASSWORD %L', :'app_pw')
 WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'gokick_app') \gexec
SELECT format('CREATE ROLE gokick_system LOGIN BYPASSRLS PASSWORD %L', :'system_pw')
 WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'gokick_system') \gexec

-- The application database, owned by the schema owner: its public schema then
-- belongs to gokick_owner (pg_database_owner), so the migrations can create in it.
SELECT 'CREATE DATABASE gokick OWNER gokick_owner'
 WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'gokick') \gexec
SQL
