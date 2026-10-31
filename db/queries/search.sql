-- name: SearchProfiles :many
WITH q AS (
    SELECT websearch_to_tsquery('russian', sqlc.arg(query_norm)::text) AS tsq
),
tag_matches AS (
    SELECT pt.profile_id,
           min(CASE pt.kind
                   WHEN 'skill' THEN 0
                   WHEN 'professional_interest' THEN 1
                   ELSE 2
               END)::int AS tag_group
    FROM profile_tag pt
    JOIN tag t ON t.id = pt.tag_id AND t.status = 'active'
    WHERE EXISTS (
        SELECT 1
        FROM tag_alias a
        WHERE a.tag_id = t.id
          AND cardinality(a.lexemes) > 0
          AND a.lexemes <@ sqlc.arg(lexemes)::text[]
    )
    GROUP BY pt.profile_id
),
fts_matches AS (
    SELECT p.user_id,
           ts_rank_cd('{0.1,0.2,0.4,1.0}', p.fts_doc, q.tsq) AS rank
    FROM profile p, q
    WHERE p.fts_doc @@ q.tsq
)
SELECT p.user_id, p.location, p.meeting_format,
       tm.tag_group,
       fm.rank,
       COALESCE((SELECT e.value::text FROM evaluation e
        WHERE e.evaluator_id = $1 AND e.target_id = p.user_id), '') AS my_evaluation
FROM profile p
JOIN users u ON u.id = p.user_id
LEFT JOIN tag_matches tm ON tm.profile_id = p.user_id
LEFT JOIN fts_matches fm ON fm.user_id = p.user_id
WHERE (tm.profile_id IS NOT NULL OR fm.user_id IS NOT NULL)
  AND p.user_id <> $1
  AND p.is_searchable
  AND u.deleted_at IS NULL
  AND (sqlc.narg(location_norm)::text IS NULL OR p.location_norm = sqlc.narg(location_norm)::text)
  AND (sqlc.narg(meeting_format)::meeting_format IS NULL
       OR p.meeting_format = sqlc.narg(meeting_format)::meeting_format
       OR p.meeting_format = 'any')
ORDER BY (tm.tag_group IS NOT NULL) DESC,
         tm.tag_group,
         fm.rank DESC NULLS LAST,
         p.user_id
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
