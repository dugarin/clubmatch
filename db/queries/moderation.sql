-- name: InsertModerationJournal :exec
INSERT INTO moderation_journal (moderator_id, action, target_type, target_id)
VALUES ($1, $2, $3, $4);

-- name: InsertNotification :exec
INSERT INTO notification (type, user_id, payload)
VALUES ($1, $2, $3);
