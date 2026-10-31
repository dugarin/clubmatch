-- name: ClaimDueNotifications :many
UPDATE notification SET attempts = attempts + 1
WHERE id IN (
    SELECT n.id FROM notification n
    WHERE n.state = 'pending' AND n.next_attempt_at <= now()
    ORDER BY n.next_attempt_at
    LIMIT 50
    FOR UPDATE SKIP LOCKED
)
RETURNING id, type::text AS type, user_id, payload, attempts, created_at;

-- name: OutboxDepth :one
SELECT count(*) FROM notification WHERE state = 'pending';

-- name: MarkNotificationSent :exec
UPDATE notification SET state = 'sent' WHERE id = $1;

-- name: MarkNotificationBlocked :exec
UPDATE notification SET state = 'blocked' WHERE id = $1;

-- name: RescheduleNotification :exec
UPDATE notification SET next_attempt_at = now() + make_interval(secs => $2) WHERE id = $1;

-- name: GetUserChatID :one
SELECT chat_id FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: ReactivateBlockedNotifications :exec
UPDATE notification SET state = 'pending', next_attempt_at = now()
WHERE state = 'blocked' AND user_id = $1;
