-- name: CreateApplication :one
INSERT INTO application (candidate_id)
VALUES ($1)
RETURNING *;

-- name: GetPendingApplicationByCandidate :one
SELECT *
FROM application
WHERE candidate_id = $1 AND state = 'pending'
ORDER BY created_at DESC LIMIT 1;

-- name: GetLatestApplicationByCandidate :one
SELECT *
FROM application
WHERE candidate_id = $1
ORDER BY created_at DESC LIMIT 1;

-- name: GetApplicationByID :one
SELECT *
FROM application
WHERE id = $1;

-- name: ListApplicationsByState :many
SELECT a.id, a.candidate_id, a.state, a.decided_by, a.decided_at, a.created_at,
       u.telegram_user_id, u.telegram_username
FROM application a
JOIN users u ON u.id = a.candidate_id
WHERE a.state = $1
ORDER BY a.created_at ASC;

-- name: DecideApplication :execrows
UPDATE application
SET state = $2, decided_by = $3, decided_at = now()
WHERE id = $1 AND state = 'pending';
