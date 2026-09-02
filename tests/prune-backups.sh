#!/bin/sh

set -eu

test_directory=$(mktemp -d)
trap 'rm -rf "$test_directory"' EXIT INT TERM

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

make_backup() {
  path=$1
  timestamp=$2
  touch -d "@$timestamp" "$path"
}

assert_exists() {
  [ -f "$1" ] || fail "expected $(basename "$1") to be retained"
}

assert_missing() {
  [ ! -e "$1" ] || fail "expected $(basename "$1") to be deleted"
}

now=$(date -u +%s)
tiered_directory="$test_directory/tiered"
mkdir "$tiered_directory"

recent_bucket=$(( (now - 3600) / 600 * 600 ))
make_backup "$tiered_directory/mongodb-recent-old.archive.gz" $((recent_bucket + 10))
make_backup "$tiered_directory/mongodb-recent-new.archive.gz" $((recent_bucket + 20))
make_backup "$tiered_directory/mongodb-recent-adjacent.archive.gz" $((recent_bucket + 610))

hourly_bucket=$(( (now - 2 * 86400) / 3600 * 3600 ))
make_backup "$tiered_directory/mongodb-hourly-old.archive.gz" $((hourly_bucket + 10))
make_backup "$tiered_directory/mongodb-hourly-new.archive.gz" $((hourly_bucket + 20))
make_backup "$tiered_directory/mongodb-hourly-adjacent.archive.gz" $((hourly_bucket + 3610))

daily_bucket=$(( (now - 10 * 86400) / 86400 * 86400 ))
make_backup "$tiered_directory/mongodb-daily-old.archive.gz" $((daily_bucket + 10))
make_backup "$tiered_directory/mongodb-daily-new.archive.gz" $((daily_bucket + 20))
make_backup "$tiered_directory/mongodb-daily-adjacent.archive.gz" $((daily_bucket + 86410))

make_backup "$tiered_directory/mongodb-expired.archive.gz" $((now - 31 * 86400))

BACKUP_DIRECTORY="$tiered_directory" \
BACKUP_RETENTION_POLICY='1d:10m,7d:1h,30d:1d' \
  /src/prune-backups

assert_missing "$tiered_directory/mongodb-recent-old.archive.gz"
assert_exists "$tiered_directory/mongodb-recent-new.archive.gz"
assert_exists "$tiered_directory/mongodb-recent-adjacent.archive.gz"
assert_missing "$tiered_directory/mongodb-hourly-old.archive.gz"
assert_exists "$tiered_directory/mongodb-hourly-new.archive.gz"
assert_exists "$tiered_directory/mongodb-hourly-adjacent.archive.gz"
assert_missing "$tiered_directory/mongodb-daily-old.archive.gz"
assert_exists "$tiered_directory/mongodb-daily-new.archive.gz"
assert_exists "$tiered_directory/mongodb-daily-adjacent.archive.gz"
assert_missing "$tiered_directory/mongodb-expired.archive.gz"

legacy_directory="$test_directory/legacy"
mkdir "$legacy_directory"
make_backup "$legacy_directory/mongodb-current.archive.gz" "$now"
make_backup "$legacy_directory/mongodb-expired.archive.gz" $((now - 9 * 86400))

BACKUP_DIRECTORY="$legacy_directory" BACKUP_RETENTION_DAYS=7 /src/prune-backups
assert_exists "$legacy_directory/mongodb-current.archive.gz"
assert_missing "$legacy_directory/mongodb-expired.archive.gz"

invalid_directory="$test_directory/invalid"
mkdir "$invalid_directory"
make_backup "$invalid_directory/mongodb-current.archive.gz" "$now"
if BACKUP_DIRECTORY="$invalid_directory" BACKUP_RETENTION_POLICY='7d:1h,1d:10m' \
  /src/prune-backups; then
  fail 'expected an invalid policy to fail'
fi
assert_exists "$invalid_directory/mongodb-current.archive.gz"

echo 'All prune-backups tests passed'
