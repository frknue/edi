-- Telegram presence: which Telegram chat speaks for which user. One chat per
-- user, one user per chat. Created by the /pair flow (pair code from the web
-- UI -> /start <code> or /pair <code> in the chat).
CREATE TABLE telegram_links (
    user_id    BIGINT PRIMARY KEY REFERENCES users(id),
    chat_id    BIGINT NOT NULL UNIQUE,
    created_at timestamptz NOT NULL
);
