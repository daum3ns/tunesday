#!/bin/sh
set -eu

SRC=/data/tunesday.db
OUT=/data/backups
DAILY_KEEP=7
WEEKLY_KEEP=4

mkdir -p "$OUT"

stamp=$(date +%F)
stamp_time=$(date +%F-%H%M)

# VACUUM INTO produces a single self-contained, WAL-checkpointed snapshot.
# Safe to run while the app is live.
python3 - "$SRC" "$OUT/tunesday-$stamp_time.db" <<'PY'
import sqlite3, sys
src, dst = sys.argv[1], sys.argv[2]
sqlite3.connect(src).execute("VACUUM INTO ?", (dst,)).close()
PY

# Sunday's backup also lands in the weekly tier.
if [ "$(date +%u)" = "7" ]; then
    cp "$OUT/tunesday-$stamp_time.db" "$OUT/tunesday-weekly-$stamp.db"
fi

# Prune daily backups beyond DAILY_KEEP. Backup filenames are ISO-stamped,
# so sorting by name is deterministic (newest last), unlike mtime.
ls -1 "$OUT"/tunesday-20*.db 2>/dev/null | grep -v weekly | sort | head -n -"$DAILY_KEEP" | xargs -r rm -f
# Prune weekly backups beyond WEEKLY_KEEP.
ls -1 "$OUT"/tunesday-weekly-*.db 2>/dev/null | sort | head -n -"$WEEKLY_KEEP" | xargs -r rm -f

echo "backup ok: $OUT/tunesday-$stamp_time.db"
