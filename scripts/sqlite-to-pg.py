#!/usr/bin/env python3
"""One-shot data migration: SQLite-era edi.db -> PostgreSQL SQL script.

Reads a SQLite backup (take one with `sqlite3 edi.db ".backup out.db"`) and
emits INSERT statements (explicit column lists — the physical column order
differs between the old ALTER-grown SQLite schema and the consolidated pg one)
plus the sequence resets. Apply with:

    python3 scripts/sqlite-to-pg.py backup.db | psql "$DATABASE_URL"

The pg schema must already exist (start the server once, or run migrations).
User 1 becomes an admin (the multi-tenant bootstrap user); token_hash stays
NULL — the server (re)binds EDI_TOKEN as user 1's token on next startup.
"""

import sqlite3
import sys

# Insert order respects foreign keys. Columns are the pg columns; the SQLite
# source must have the same names (it does — only order/types differ).
TABLES = [
    ("users", ["id", "name", "created_at"]),
    ("attributes", ["id", "user_id", "key", "name", "total_xp", "peak_xp", "created_at"]),
    ("quests", ["id", "user_id", "title", "description", "type", "difficulty", "status",
                "attribute_rewards", "skip_count", "source_suggestion_id", "created_at",
                "completed_at", "due_date"]),
    ("quest_subtasks", ["id", "user_id", "quest_id", "title", "attribute_rewards", "done", "created_at"]),
    ("quest_completions", ["id", "user_id", "quest_id", "xp_awarded", "completed_at"]),
    ("xp_events", ["id", "user_id", "attribute_key", "amount", "source", "source_id", "note", "created_at"]),
    ("streaks", ["id", "user_id", "current_count", "longest_count", "last_active_date"]),
    ("journal_entries", ["id", "user_id", "mood", "energy", "notes", "created_at"]),
    ("agent_suggestions", ["id", "user_id", "type", "title", "reason", "suggested_quest", "status",
                           "created_quest_id", "source_quest_id", "created_at", "resolved_at"]),
    ("openai_credentials", ["user_id", "access_token", "refresh_token", "id_token", "account_id",
                            "email", "expires_at", "updated_at"]),
    ("app_settings", ["user_id", "key", "value"]),
    ("tool_entries", ["id", "user_id", "tool_key", "data", "summary", "xp_awarded", "created_at"]),
    ("gold_events", ["id", "user_id", "amount", "source", "label", "shop_item_id", "created_at"]),
    ("shop_items", ["id", "user_id", "name", "price", "created_at", "archived_at"]),
    ("wards", ["id", "user_id", "attribute_key", "expires_at", "created_at"]),
]

# Tables with an identity id column whose sequence needs resetting after
# explicit-id inserts.
SERIAL_TABLES = [t for t, cols in TABLES if "id" in cols]


def quote(v):
    if v is None:
        return "NULL"
    if isinstance(v, (int, float)):
        return str(v)
    return "'" + str(v).replace("'", "''") + "'"


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: sqlite-to-pg.py <backup.db>")
    con = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
    con.row_factory = sqlite3.Row

    print("BEGIN;")
    # Full restore: the target's data (e.g. the blank user the server
    # bootstrapped on first start) is replaced wholesale.
    print("TRUNCATE " + ", ".join(t for t, _ in TABLES) + " RESTART IDENTITY CASCADE;")
    for table, cols in TABLES:
        rows = con.execute(f"SELECT {', '.join(cols)} FROM {table} ORDER BY rowid").fetchall()
        for r in rows:
            vals = ", ".join(quote(r[c]) for c in cols)
            print(f"INSERT INTO {table} ({', '.join(cols)}) VALUES ({vals});")
        print(f"-- {table}: {len(rows)} rows")
    # The original single user becomes the multi-tenant admin.
    print("UPDATE users SET is_admin = TRUE WHERE id = 1;")
    for table in SERIAL_TABLES:
        print(
            f"SELECT setval(pg_get_serial_sequence('{table}','id'), "
            f"(SELECT COALESCE(MAX(id),0)+1 FROM {table}), false);"
        )
    print("COMMIT;")


if __name__ == "__main__":
    main()
