-- name: UpsertUserByTelegramID :one
INSERT INTO users (telegram_user_id, telegram_username)
VALUES ($1, $2)
ON CONFLICT (telegram_user_id) DO UPDATE
SET telegram_username = EXCLUDED.telegram_username
RETURNING *;

-- name: SetUserChatID :exec
UPDATE users SET chat_id = $2 WHERE telegram_user_id = $1;

-- name: GetUserByTelegramID :one
SELECT * FROM users WHERE telegram_user_id = $1 AND deleted_at IS NULL;

-- name: SetUserOnboarded :exec
UPDATE users
SET access_state = 'onboarded',
    role = CASE WHEN role = 'candidate' THEN 'member'::user_role ELSE role END
WHERE id = $1;
