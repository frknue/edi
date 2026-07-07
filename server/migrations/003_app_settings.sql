-- 003_app_settings.sql — small key/value store for per-user preferences
-- (e.g. the AI reasoning effort and model choice).

CREATE TABLE app_settings (
    user_id INTEGER NOT NULL REFERENCES users(id),
    key     TEXT NOT NULL,
    value   TEXT NOT NULL,
    PRIMARY KEY (user_id, key)
);
