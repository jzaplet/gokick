# Postgres 18 for development, the test suite and CI — one image, so all three run
# the same server and the same role bootstrap. Used by the db and db-test services
# of docker-compose.yml; production points APP_DB_*_URL at its own server instead.
#
# The Debian variant (not alpine) is deliberate: it ships the ICU data behind the
# app_sort collation (cs-CZ) the Postgres migrations create.
FROM postgres:18

# Runs once, on an empty data directory: creates gokick_owner / gokick_app /
# gokick_system and the gokick database (see the script for what each role is).
COPY docker/postgres/initdb/ /docker-entrypoint-initdb.d/

# Ready means the FINAL server accepts TCP connections. The entrypoint runs the
# init scripts on a temporary server that listens on the Unix socket only, so a
# socket check would report ready while the roles are still being created;
# `docker compose up --wait` must not return before the bootstrap is done. It is
# probed every second while the server starts (so --wait returns promptly), then
# every 30 s for as long as the container runs.
HEALTHCHECK --start-period=60s --start-interval=1s --interval=30s --timeout=3s --retries=3 \
	CMD pg_isready -h 127.0.0.1 -p 5432 -U "${POSTGRES_USER:-postgres}" || exit 1
