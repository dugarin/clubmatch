-- name: ListDeckCards :many
WITH viewer_tags AS (
    SELECT pt.tag_id FROM profile_tag pt WHERE pt.profile_id = $1
),
ranked AS (
    SELECT p.user_id, p.location, p.meeting_format,
        CASE
            WHEN EXISTS (
                SELECT 1
                FROM profile_tag pt
                JOIN viewer_tags vt ON vt.tag_id = pt.tag_id
                WHERE pt.profile_id = p.user_id
                  AND pt.kind IN ('skill', 'professional_interest')
            ) THEN 0
            WHEN EXISTS (
                SELECT 1
                FROM profile_tag pt
                JOIN viewer_tags vt ON vt.tag_id = pt.tag_id
                WHERE pt.profile_id = p.user_id
            ) THEN 1
            ELSE 2
        END AS match_group
    FROM profile p
    JOIN users u ON u.id = p.user_id
    WHERE p.user_id <> $1
      AND p.is_searchable
      AND u.deleted_at IS NULL
      AND NOT EXISTS (
          SELECT 1
          FROM evaluation e
          WHERE e.evaluator_id = $1
            AND e.target_id = p.user_id
            AND e.updated_at > now() - interval '30 days'
      )
)
SELECT user_id, location, meeting_format
FROM ranked
ORDER BY match_group, random()
LIMIT $2;
