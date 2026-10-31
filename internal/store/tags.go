package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrTagRevoked — значение совпало с отозванным модерацией тегом (D-3.2, вариант б).
var ErrTagRevoked = errors.New("tag rejected by moderation")

// GetOrCreateTag resolves a raw user-entered value to an existing tag
// (by normalized form or by stemmed lexemes of canonical/aliases) or creates
// a new active tag with the value as its first alias. Revoked tags are not
// reused: resolving to one yields ErrTagRevoked (D-3.2).
func GetOrCreateTag(ctx context.Context, q *Queries, createdBy pgtype.UUID, raw, normalized string) (Tag, error) {
	lexemes, err := q.LexemesOfText(ctx, normalized)
	if err != nil {
		return Tag{}, err
	}
	t, err := q.FindTagByNormalizedOrLexemes(ctx, FindTagByNormalizedOrLexemesParams{
		Normalized: normalized,
		Column2:    lexemes,
	})
	if err == nil {
		if t.Status == TagStatusRevoked {
			return Tag{}, ErrTagRevoked
		}
		return t, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Tag{}, err
	}
	t, err = q.CreateTag(ctx, CreateTagParams{
		Canonical:  strings.TrimSpace(raw),
		Normalized: normalized,
		CreatedBy:  createdBy,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Concurrent insert won the unique(normalized) race.
		t, err = q.GetTagByNormalized(ctx, normalized)
		if err == nil && t.Status == TagStatusRevoked {
			return Tag{}, ErrTagRevoked
		}
	}
	if err != nil {
		return Tag{}, err
	}
	if err := q.CreateTagAlias(ctx, CreateTagAliasParams{
		TagID:      t.ID,
		Alias:      strings.TrimSpace(raw),
		Normalized: normalized,
		Lexemes:    lexemes,
	}); err != nil {
		return Tag{}, err
	}
	return t, nil
}
