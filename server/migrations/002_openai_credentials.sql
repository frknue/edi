-- 002_openai_credentials.sql — stores the user's ChatGPT-subscription OAuth
-- tokens so AI features run against their own OpenAI account. One row per user.
-- Tokens are sensitive; the DB file (edi.db) is gitignored and single-user.

CREATE TABLE openai_credentials (
    user_id       INTEGER PRIMARY KEY REFERENCES users(id),
    access_token  TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    id_token      TEXT NOT NULL DEFAULT '',
    account_id    TEXT NOT NULL DEFAULT '',
    email         TEXT NOT NULL DEFAULT '',
    expires_at    TEXT NOT NULL,      -- RFC3339 UTC; access token expiry
    updated_at    TEXT NOT NULL
);
