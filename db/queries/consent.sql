-- name: InsertConsent :exec
INSERT INTO consent (user_id, document_version)
VALUES ($1, $2)
ON CONFLICT (user_id, document_version) DO NOTHING;
