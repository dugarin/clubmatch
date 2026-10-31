-- name: DeleteEvaluationsByUser :exec
DELETE FROM evaluation WHERE evaluator_id = $1 OR target_id = $1;

-- name: DeleteEvaluationHistoryByUser :exec
DELETE FROM evaluation_history WHERE evaluator_id = $1 OR target_id = $1;

-- name: DeleteProfileTagsByUser :exec
DELETE FROM profile_tag WHERE profile_id = $1;

-- name: DeleteProjectsByUser :exec
DELETE FROM project WHERE profile_id = $1;

-- name: DeleteConsentByUser :exec
DELETE FROM consent WHERE user_id = $1;

-- name: DeleteProfileByUser :execrows
DELETE FROM profile WHERE user_id = $1;

-- name: MarkMatchesUnavailableByUser :exec
UPDATE match SET state = 'contact_unavailable'
WHERE (user_low = $1 OR user_high = $1) AND state = 'active';

-- name: CancelPendingNotificationsByUser :exec
UPDATE notification SET state = 'cancelled'
WHERE user_id = $1 AND state = 'pending';

-- name: SoftDeleteUser :exec
UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;
