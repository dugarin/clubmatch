-- name: UpsertUserByTelegramID :one
INSERT INTO users (telegram_user_id, telegram_username)
VALUES ($1, $2)
ON CONFLICT (telegram_user_id) DO UPDATE
SET telegram_username = EXCLUDED.telegram_username
RETURNING *;

-- name: SetUserChatID :exec
UPDATE users SET chat_id = $2 WHERE telegram_user_id = $1;
