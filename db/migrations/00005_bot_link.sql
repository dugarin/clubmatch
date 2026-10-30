-- +goose Up
ALTER TABLE users ADD COLUMN chat_id bigint;

CREATE TABLE bot_link_nonce (
    nonce            text PRIMARY KEY,
    telegram_user_id bigint NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    used             boolean NOT NULL DEFAULT false
);

-- +goose Down
DROP TABLE bot_link_nonce;
ALTER TABLE users DROP COLUMN chat_id;
