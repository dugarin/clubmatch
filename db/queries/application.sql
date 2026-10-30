-- name: CreateApplication :one
INSERT INTO application (candidate_id)
VALUES ($1)
RETURNING *;

-- name: GetPendingApplicationByCandidate :one
SELECT *
FROM application
WHERE candidate_id = $1 AND state = 'pending'
ORDER BY created_at DESC LIMIT 1;
