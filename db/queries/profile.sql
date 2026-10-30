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

-- name: GetProfileSelf :one
SELECT p.user_id, p.first_name, p.last_name, p.location, p.meeting_format, p.is_searchable, p.updated_at,
       (u.chat_id IS NOT NULL)::bool AS bot_linked
FROM profile p
JOIN users u ON u.id = p.user_id
WHERE p.user_id = $1;

-- name: DeleteProfileTagsByKind :exec
DELETE FROM profile_tag WHERE profile_id = $1 AND kind = $2;

-- name: InsertProfileTag :exec
INSERT INTO profile_tag (profile_id, tag_id, kind)
VALUES ($1, $2, $3)
ON CONFLICT (profile_id, tag_id, kind) DO NOTHING;

-- name: ListProfileTagCanonicals :many
SELECT t.canonical
FROM profile_tag pt
JOIN tag t ON t.id = pt.tag_id
WHERE pt.profile_id = $1 AND pt.kind = $2 AND t.status = 'active'
ORDER BY t.canonical;

-- name: DeleteProjectsByProfileID :exec
DELETE FROM project WHERE profile_id = $1;

-- name: InsertProject :exec
INSERT INTO project (profile_id, text, position) VALUES ($1, $2, $3);

-- name: ListProjectTexts :many
SELECT text FROM project WHERE profile_id = $1 ORDER BY position;

-- name: RebuildProfileFTS :exec
UPDATE profile p
SET fts_doc =
    setweight(to_tsvector('russian', coalesce((
        SELECT string_agg(t.canonical, ' ')
        FROM profile_tag pt JOIN tag t ON t.id = pt.tag_id
        WHERE pt.profile_id = p.user_id AND pt.kind = 'skill' AND t.status = 'active'), '')), 'A') ||
    setweight(to_tsvector('russian', coalesce((
        SELECT string_agg(pr.text, ' ') FROM project pr
        WHERE pr.profile_id = p.user_id), '')), 'B') ||
    setweight(to_tsvector('russian', coalesce((
        SELECT string_agg(t.canonical, ' ')
        FROM profile_tag pt JOIN tag t ON t.id = pt.tag_id
        WHERE pt.profile_id = p.user_id AND pt.kind = 'professional_interest' AND t.status = 'active'), '')), 'C') ||
    setweight(to_tsvector('russian', coalesce((
        SELECT string_agg(t.canonical, ' ')
        FROM profile_tag pt JOIN tag t ON t.id = pt.tag_id
        WHERE pt.profile_id = p.user_id AND pt.kind = 'personal_interest' AND t.status = 'active'), '')), 'D')
WHERE p.user_id = $1;
