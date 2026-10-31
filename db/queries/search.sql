-- name: ResolveTagIDsByLexemes :many
SELECT DISTINCT t.id
FROM tag t
WHERE t.status = 'active'
  AND EXISTS (
      SELECT 1
      FROM tag_alias a
      WHERE a.tag_id = t.id
        AND cardinality(a.lexemes) > 0
        AND a.lexemes <@ $1::text[]
  );

-- name: SearchProfilesByTags :many
WITH matched AS (
    SELECT unnest(sqlc.arg(tag_ids)::uuid[]) AS tag_id
),
ranked AS (
    SELECT pt.profile_id,
           min(CASE pt.kind
                   WHEN 'skill' THEN 0
                   WHEN 'professional_interest' THEN 1
                   ELSE 2
               END) FILTER (WHERE pt.tag_id IN (SELECT tag_id FROM matched))::int AS tag_group
    FROM profile_tag pt
    GROUP BY pt.profile_id
    HAVING count(DISTINCT pt.tag_id) FILTER (WHERE pt.tag_id IN (SELECT tag_id FROM matched))
           = cardinality(sqlc.arg(tag_ids)::uuid[])
)
SELECT p.user_id, p.location, p.meeting_format,
       r.tag_group,
       COALESCE((SELECT e.value::text FROM evaluation e
        WHERE e.evaluator_id = $1 AND e.target_id = p.user_id), '') AS my_evaluation
FROM ranked r
JOIN profile p ON p.user_id = r.profile_id
JOIN users u ON u.id = p.user_id
WHERE p.user_id <> $1
  AND p.is_searchable
  AND u.deleted_at IS NULL
  AND (sqlc.narg(location_norm)::text IS NULL OR p.location_norm = sqlc.narg(location_norm)::text)
  AND (sqlc.narg(meeting_format)::meeting_format IS NULL
       OR p.meeting_format = sqlc.narg(meeting_format)::meeting_format
       OR p.meeting_format = 'any')
ORDER BY r.tag_group, p.user_id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
