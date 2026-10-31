package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type store_UserRef struct {
	id   pgtype.UUID
	tgID int64
}

type matchTestItem struct {
	MatchID string         `json:"match_id"`
	State   string         `json:"state"`
	Card    map[string]any `json:"card"`
}

func makeMatch(t *testing.T, e *testEnv, a, b *store_UserRef) {
	t.Helper()
	for _, pair := range [][2]*store_UserRef{{a, b}, {b, a}} {
		rec := call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
			map[string]any{"target_id": uuidString(pair[1].id), "value": "interested"}, pair[0].tgID)
		if rec.Code != http.StatusCreated {
			t.Fatalf("evaluate: %d %s", rec.Code, rec.Body.String())
		}
	}
}

func listMatches(t *testing.T, e *testEnv, tgID int64) []matchTestItem {
	t.Helper()
	rec := call(e.mth.List, http.MethodGet, "/api/v1/matches", nil, tgID)
	if rec.Code != http.StatusOK {
		t.Fatalf("list matches: %d %s", rec.Code, rec.Body.String())
	}
	var out []matchTestItem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	return out
}

func TestMatchesDiscloseContactAndJournal(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := mkProfile(t, e, map[string]any{"skills": []string{"MatchA" + runID}})
	b := mkProfile(t, e, map[string]any{"skills": []string{"MatchB" + runID}})

	ua := &store_UserRef{id: a.ID, tgID: a.TelegramUserID}
	ub := &store_UserRef{id: b.ID, tgID: b.TelegramUserID}
	makeMatch(t, e, ua, ub)

	items := listMatches(t, e, a.TelegramUserID)
	if len(items) != 1 {
		t.Fatalf("matches: %d", len(items))
	}
	m := items[0]
	if m.State != "active" || m.Card == nil {
		t.Fatalf("match item: %+v", m)
	}
	if m.Card["telegram_user_id"] == nil {
		t.Errorf("contact must be disclosed after match (FR-8): %v", m.Card)
	}

	var journalCount int
	if err := e.pool.QueryRow(ctx,
		"SELECT count(*) FROM contact_disclosure_journal WHERE match_id = $1 AND recipient_id = $2",
		m.MatchID, a.ID).Scan(&journalCount); err != nil {
		t.Fatal(err)
	}
	if journalCount < 1 {
		t.Error("disclosure must be journaled on each issuance (Q-4.1)")
	}
	listMatches(t, e, a.TelegramUserID)
	if err := e.pool.QueryRow(ctx,
		"SELECT count(*) FROM contact_disclosure_journal WHERE match_id = $1 AND recipient_id = $2",
		m.MatchID, a.ID).Scan(&journalCount); err != nil {
		t.Fatal(err)
	}
	if journalCount < 2 {
		t.Errorf("each issuance must append journal entry, got %d", journalCount)
	}
}

func TestMatchesContactUnavailable(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := mkProfile(t, e, map[string]any{"skills": []string{"DelMatchA" + runID}})
	b := mkProfile(t, e, map[string]any{"skills": []string{"DelMatchB" + runID}})
	makeMatch(t, e,
		&store_UserRef{id: a.ID, tgID: a.TelegramUserID},
		&store_UserRef{id: b.ID, tgID: b.TelegramUserID})

	if _, err := e.pool.Exec(ctx,
		"UPDATE match SET state = 'contact_unavailable' WHERE user_low = least($1::uuid, $2::uuid) AND user_high = greatest($1::uuid, $2::uuid)",
		a.ID, b.ID); err != nil {
		t.Fatal(err)
	}

	items := listMatches(t, e, a.TelegramUserID)
	if len(items) != 1 {
		t.Fatalf("matches: %d", len(items))
	}
	if items[0].State != "contact_unavailable" || items[0].Card != nil {
		t.Errorf("deleted side must yield card=null (FR-17): %+v", items[0])
	}
}
