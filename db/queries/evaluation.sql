-- name: LockEvaluationPair :exec
SELECT pg_advisory_xact_lock(hashtext(sqlc.arg(user_low)::text || ':' || sqlc.arg(user_high)::text));

-- name: UpsertEvaluation :exec
INSERT INTO evaluation (evaluator_id, target_id, value, source)
VALUES ($1, $2, $3, $4)
ON CONFLICT (evaluator_id, target_id) DO UPDATE
SET value = EXCLUDED.value,
    source = EXCLUDED.source,
    updated_at = now();

-- name: InsertEvaluationHistory :exec
INSERT INTO evaluation_history (evaluator_id, target_id, value)
VALUES ($1, $2, $3);

-- name: GetEvaluationValue :one
SELECT value FROM evaluation WHERE evaluator_id = $1 AND target_id = $2;

-- name: CreateMatch :one
INSERT INTO match (user_low, user_high)
VALUES ($1, $2)
ON CONFLICT (user_low, user_high) DO NOTHING
RETURNING id;

-- name: GetMatchByPair :one
SELECT id FROM match WHERE user_low = $1 AND user_high = $2;

-- name: InsertContactDisclosure :exec
INSERT INTO contact_disclosure_journal
    (match_id, subject_id, recipient_id, subject_telegram_user_id, recipient_telegram_user_id)
VALUES ($1, $2, $3, $4, $5);

-- name: GetCardContact :one
SELECT u.id, u.telegram_user_id, u.telegram_username,
       p.first_name, p.last_name, p.location, p.meeting_format
FROM users u
JOIN profile p ON p.user_id = u.id
WHERE u.id = $1 AND u.deleted_at IS NULL;
