-- name: EnsureDevUser :one
INSERT INTO users (telegram_user_id, telegram_username, role, access_state)
VALUES ($1, $2, 'member', 'onboarded')
ON CONFLICT (telegram_user_id) DO UPDATE
SET telegram_username = EXCLUDED.telegram_username,
    role = 'member',
    access_state = 'onboarded',
    deleted_at = NULL
RETURNING *;

-- name: EnsureDevConsent :exec
INSERT INTO consent (user_id, document_version)
VALUES ($1, 'dev-preview')
ON CONFLICT (user_id, document_version) DO NOTHING;

-- name: EnsureDevTag :one
INSERT INTO tag (canonical, normalized, created_by)
VALUES ($1, $2, $3)
ON CONFLICT (normalized) DO UPDATE
SET canonical = EXCLUDED.canonical,
    status = 'active',
    created_by = EXCLUDED.created_by
RETURNING *;

-- name: EnsureDevProject :exec
INSERT INTO project (profile_id, text, position)
SELECT $1, $2, $3
WHERE NOT EXISTS (
    SELECT 1 FROM project WHERE profile_id = $1 AND position = $3
);

-- name: EnsureDevMatch :exec
INSERT INTO match (user_low, user_high)
VALUES ($1, $2)
ON CONFLICT (user_low, user_high) DO UPDATE SET state = 'active';
