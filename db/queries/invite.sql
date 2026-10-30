-- name: CreateInvite :one
INSERT INTO invite (token, created_by)
VALUES ($1, $2)
RETURNING *;

-- name: GetInviteByToken :one
SELECT *
FROM invite WHERE token = $1;

-- name: MarkInviteUsed :execrows
UPDATE invite SET used_by = $2, used_at = now()
WHERE token = $1 AND used_by IS NULL;
