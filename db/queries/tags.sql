-- name: LexemesOfText :one
SELECT tsvector_to_array(to_tsvector('russian', $1::text))::text[] AS lexemes;

-- name: FindTagByNormalizedOrLexemes :one
SELECT t.id, t.canonical, t.normalized, t.status, t.created_by
FROM tag t
WHERE t.status = 'active'
  AND (t.normalized = $1
       OR EXISTS (SELECT 1 FROM tag_alias a
                  WHERE a.tag_id = t.id
                    AND (a.normalized = $1
                         OR (cardinality($2::text[]) > 0 AND a.lexemes = $2::text[]))))
ORDER BY (t.normalized = $1) DESC
LIMIT 1;

-- name: GetTagByNormalized :one
SELECT * FROM tag WHERE normalized = $1;

-- name: CreateTag :one
INSERT INTO tag (canonical, normalized, created_by)
VALUES ($1, $2, $3)
ON CONFLICT (normalized) DO NOTHING
RETURNING *;

-- name: CreateTagAlias :exec
INSERT INTO tag_alias (tag_id, alias, normalized, lexemes)
VALUES ($1, $2, $3, $4)
ON CONFLICT (normalized) DO NOTHING;

-- name: AutocompleteTags :many
SELECT t.id, t.canonical
FROM tag t
WHERE t.status = 'active'
  AND (t.normalized % $1
       OR t.normalized LIKE $1 || '%'
       OR EXISTS (SELECT 1 FROM tag_alias a
                  WHERE a.tag_id = t.id
                    AND (a.normalized % $1 OR a.normalized LIKE $1 || '%')))
ORDER BY greatest(
           similarity(t.normalized, $1),
           coalesce((SELECT max(similarity(a.normalized, $1))
                     FROM tag_alias a WHERE a.tag_id = t.id), 0::real)
         ) DESC,
         t.canonical ASC
LIMIT 10;
