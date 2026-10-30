-- name: GetProfileByUserID :one
SELECT user_id, first_name, last_name, location, location_norm, meeting_format, is_searchable, updated_at
FROM profile WHERE user_id = $1;

-- name: UpsertProfile :exec
INSERT INTO profile (user_id, first_name, last_name, location, location_norm, meeting_format, is_searchable)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id) DO UPDATE
SET first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    location = EXCLUDED.location,
    location_norm = EXCLUDED.location_norm,
    meeting_format = EXCLUDED.meeting_format,
    is_searchable = EXCLUDED.is_searchable,
    updated_at = now();
