#!/usr/bin/env bash
set -euo pipefail

CONTAINER="${PGTEST_CONTAINER:-im-thread-pgtest}"
IMAGE="${PGTEST_IMAGE:-postgres:16}"
HOST_PORT="${PGTEST_PORT:-5433}"
PASSWORD="postgres"
DB="postgres"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

cleanup

echo ">> postgres ($IMAGE) :$HOST_PORT"
docker run --rm -d \
  --name "$CONTAINER" \
  -e POSTGRES_PASSWORD="$PASSWORD" \
  -e POSTGRES_DB="$DB" \
  -p "$HOST_PORT:5432" \
  "$IMAGE" >/dev/null

echo ">> waiting"
for i in $(seq 1 60); do
  if docker exec "$CONTAINER" pg_isready -U postgres -d "$DB" >/dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "!! postgres not ready" >&2
    exit 1
  fi
  sleep 1
done

export POSTGRES_DSN="postgres://postgres:${PASSWORD}@localhost:${HOST_PORT}/${DB}?sslmode=disable"
echo ">> POSTGRES_DSN=$POSTGRES_DSN"

cd "$ROOT"

echo ">> unit"
go test ./... 2>&1 | grep -vE '\[no test files\]' || true

echo ">> integration"
go test -tags=integration -count=1 \
  ./internal/adapter/journal/... \
  ./internal/store/postgres/...

echo ">> ok"
