package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

func evaluate(t *testing.T, e *testEnv, viewer store.User, targetID pgtype.UUID, value string) *httptest.ResponseRecorder {
	t.Helper()
	return call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": uuidString(targetID), "value": value, "source": "deck"},
		viewer.TelegramUserID)
}

func countRows(t *testing.T, e *testEnv, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestEvaluationNoReciprocal(t *testing.T) {
	e := newTestEnv(t)
	a := mkProfile(t, e, nil)
	b := mkProfile(t, e, nil)

	rec := evaluate(t, e, a, b.ID, "interested")
	if rec.Code != http.StatusCreated {
		t.Fatalf("evaluate: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["match"] != false {
		t.Errorf("match = %v, want false", resp["match"])
	}
	if _, ok := resp["card"]; ok {
		t.Error("card must be absent without match (FR-7)")
	}
	// оценка и первая запись истории зафиксированы (Q-5.1)
	if n := countRows(t, e, "SELECT count(*) FROM evaluation WHERE evaluator_id = $1 AND target_id = $2 AND value = 'interested'", a.ID, b.ID); n != 1 {
		t.Errorf("evaluation rows = %d, want 1", n)
	}
	if n := countRows(t, e, "SELECT count(*) FROM evaluation_history WHERE evaluator_id = $1 AND target_id = $2", a.ID, b.ID); n != 1 {
		t.Errorf("history rows = %d, want 1", n)
	}
	if n := countRows(t, e, "SELECT count(*) FROM match WHERE (user_low = $1 OR user_high = $1) AND (user_low = $2 OR user_high = $2)", a.ID, b.ID); n != 0 {
		t.Errorf("match rows = %d, want 0", n)
	}
}

func TestEvaluationMatch(t *testing.T) {
	e := newTestEnv(t)
	a := mkProfile(t, e, map[string]any{"skills": []string{"MatchA" + runID}})
	b := mkProfile(t, e, map[string]any{"skills": []string{"MatchB" + runID}})

	if rec := evaluate(t, e, a, b.ID, "interested"); rec.Code != http.StatusCreated {
		t.Fatalf("A->B: %d %s", rec.Code, rec.Body.String())
	}
	rec := evaluate(t, e, b, a.ID, "interested")
	if rec.Code != http.StatusCreated {
		t.Fatalf("B->A: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Match bool           `json:"match"`
		Card  map[string]any `json:"card"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Match {
		t.Fatal("match = false, want true")
	}
	// карточка after_match: контакт A раскрыт B (FR-8)
	if resp.Card["telegram_user_id"] != float64(a.TelegramUserID) {
		t.Errorf("card.telegram_user_id = %v, want %d", resp.Card["telegram_user_id"], a.TelegramUserID)
	}
	if resp.Card["first_name"] != "Иван" {
		t.Errorf("card.first_name = %v", resp.Card["first_name"])
	}
	if skills, ok := resp.Card["skills"].([]any); !ok || len(skills) != 1 || skills[0] != "MatchA"+runID {
		t.Errorf("card.skills = %v", resp.Card["skills"])
	}
	if _, ok := resp.Card["avatar_url"]; !ok {
		t.Error("card.avatar_url must be present (placeholder)")
	}
	if _, ok := resp.Card["telegram_username"]; !ok {
		t.Error("card.telegram_username must be present (nullable)")
	}

	var matchID pgtype.UUID
	low, high := orderedPair(a.ID, b.ID)
	if err := e.pool.QueryRow(context.Background(),
		"SELECT id FROM match WHERE user_low = $1 AND user_high = $2", low, high).Scan(&matchID); err != nil {
		t.Fatalf("match row: %v", err)
	}
	// журнал раскрытия ×2 со снапшотами telegram_user_id (ADR-004 п.4)
	if n := countRows(t, e, "SELECT count(*) FROM contact_disclosure_journal WHERE match_id = $1", matchID); n != 2 {
		t.Errorf("disclosure journal rows = %d, want 2", n)
	}
	if n := countRows(t, e,
		"SELECT count(*) FROM contact_disclosure_journal WHERE match_id = $1 AND subject_id = $2 AND recipient_id = $3 AND subject_telegram_user_id = $4 AND recipient_telegram_user_id = $5",
		matchID, a.ID, b.ID, a.TelegramUserID, b.TelegramUserID); n != 1 {
		t.Errorf("journal A->B row missing or wrong snapshots")
	}
	// outbox ×2 (match_created, payload {match_id})
	if n := countRows(t, e,
		"SELECT count(*) FROM notification WHERE type = 'match_created' AND user_id IN ($1, $2) AND payload->>'match_id' = $3",
		a.ID, b.ID, uuidString(matchID)); n != 2 {
		t.Errorf("outbox rows = %d, want 2", n)
	}
}

// FR-12/AC-5: смена оценки; повторная оценка пишет историю; снятие «интересно»
// после матча не отзывает раскрытый контакт.
func TestEvaluationChangeAndUnmatchKeepsContact(t *testing.T) {
	e := newTestEnv(t)
	a := mkProfile(t, e, nil)
	b := mkProfile(t, e, nil)

	evaluate(t, e, a, b.ID, "interested")
	// смена interested -> not_interested до взаимности: матч не создаётся (AC-5)
	rec := evaluate(t, e, a, b.ID, "not_interested")
	if rec.Code != http.StatusCreated {
		t.Fatalf("change: %d %s", rec.Code, rec.Body.String())
	}
	if n := countRows(t, e, "SELECT count(*) FROM evaluation_history WHERE evaluator_id = $1 AND target_id = $2", a.ID, b.ID); n != 2 {
		t.Errorf("history rows = %d, want 2 (каждая оценка, Q-5.1)", n)
	}
	var val string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT value FROM evaluation WHERE evaluator_id = $1 AND target_id = $2", a.ID, b.ID).Scan(&val); err != nil {
		t.Fatal(err)
	}
	if val != "not_interested" {
		t.Errorf("evaluation value = %s, want not_interested", val)
	}
	if rec := evaluate(t, e, b, a.ID, "interested"); rec.Code != http.StatusCreated {
		t.Fatalf("B->A: %d", rec.Code)
	}
	low, high := orderedPair(a.ID, b.ID)
	if n := countRows(t, e, "SELECT count(*) FROM match WHERE user_low = $1 AND user_high = $2", low, high); n != 0 {
		t.Error("match must not be created while A is not_interested")
	}

	// взаимность и матч
	evaluate(t, e, a, b.ID, "interested")
	if n := countRows(t, e, "SELECT count(*) FROM match WHERE user_low = $1 AND user_high = $2", low, high); n != 1 {
		t.Fatalf("match rows = %d, want 1", n)
	}
	var matchID pgtype.UUID
	if err := e.pool.QueryRow(context.Background(),
		"SELECT id FROM match WHERE user_low = $1 AND user_high = $2", low, high).Scan(&matchID); err != nil {
		t.Fatal(err)
	}

	// снятие «интересно» после матча: матч и журнал сохраняются (FR-12)
	rec = evaluate(t, e, a, b.ID, "not_interested")
	if rec.Code != http.StatusCreated {
		t.Fatalf("unmatch: %d %s", rec.Code, rec.Body.String())
	}
	if n := countRows(t, e, "SELECT count(*) FROM match WHERE id = $1", matchID); n != 1 {
		t.Error("match revoked after un-interested (FR-12 violated)")
	}
	if n := countRows(t, e, "SELECT count(*) FROM contact_disclosure_journal WHERE match_id = $1", matchID); n != 2 {
		t.Errorf("journal rows = %d after un-interested, want 2", n)
	}

	// повторное «интересно» при существующем матче: без дублей журнала/outbox
	rec = evaluate(t, e, a, b.ID, "interested")
	if rec.Code != http.StatusCreated {
		t.Fatalf("re-interested: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["match"] != true {
		t.Errorf("re-interested: match = %v, want true (матч существует)", resp["match"])
	}
	if n := countRows(t, e, "SELECT count(*) FROM match WHERE id = $1", matchID); n != 1 {
		t.Error("duplicate match created")
	}
	if n := countRows(t, e, "SELECT count(*) FROM contact_disclosure_journal WHERE match_id = $1", matchID); n != 2 {
		t.Errorf("journal rows = %d after re-interested, want 2 (no duplicates)", n)
	}
	if n := countRows(t, e, "SELECT count(*) FROM notification WHERE type = 'match_created' AND user_id IN ($1, $2)", a.ID, b.ID); n != 2 {
		t.Errorf("outbox rows = %d after re-interested, want 2 (no duplicates)", n)
	}
}

// S1/F13: гонка взаимных оценок — ровно один матч, журнал ×2, outbox ×2.
func TestEvaluationMatchRace(t *testing.T) {
	e := newTestEnv(t)
	a := mkProfile(t, e, nil)
	b := mkProfile(t, e, nil)

	var wg sync.WaitGroup
	recs := make([]*httptest.ResponseRecorder, 2)
	for i, pair := range [][2]store.User{{a, b}, {b, a}} {
		wg.Add(1)
		go func(i int, viewer, target store.User) {
			defer wg.Done()
			recs[i] = evaluate(t, e, viewer, target.ID, "interested")
		}(i, pair[0], pair[1])
	}
	wg.Wait()
	for i, rec := range recs {
		if rec.Code != http.StatusCreated {
			t.Fatalf("goroutine %d: status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}
	low, high := orderedPair(a.ID, b.ID)
	if n := countRows(t, e, "SELECT count(*) FROM match WHERE user_low = $1 AND user_high = $2", low, high); n != 1 {
		t.Errorf("match rows = %d, want 1", n)
	}
	var matchID pgtype.UUID
	if err := e.pool.QueryRow(context.Background(),
		"SELECT id FROM match WHERE user_low = $1 AND user_high = $2", low, high).Scan(&matchID); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, e, "SELECT count(*) FROM contact_disclosure_journal WHERE match_id = $1", matchID); n != 2 {
		t.Errorf("journal rows = %d, want 2", n)
	}
	if n := countRows(t, e, "SELECT count(*) FROM notification WHERE type = 'match_created' AND user_id IN ($1, $2)", a.ID, b.ID); n != 2 {
		t.Errorf("outbox rows = %d, want 2", n)
	}
}

func TestEvaluationValidation(t *testing.T) {
	e := newTestEnv(t)
	a := mkProfile(t, e, nil)
	b := mkProfile(t, e, nil)

	// target = self
	rec := evaluate(t, e, a, a.ID, "interested")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("self evaluation: status = %d, want 422", rec.Code)
	}
	// несуществующий uuid
	rec = call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": "00000000-0000-0000-0000-000000000000", "value": "interested"},
		a.TelegramUserID)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown target: status = %d, want 404", rec.Code)
	}
	// пользователь без анкеты — не участник
	noProfile := e.createUser(t, "approved")
	rec = evaluate(t, e, a, noProfile.ID, "interested")
	if rec.Code != http.StatusNotFound {
		t.Errorf("target without profile: status = %d, want 404", rec.Code)
	}
	// невалидные значения
	rec = call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": uuidString(b.ID), "value": "maybe"}, a.TelegramUserID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad value: status = %d, want 422", rec.Code)
	}
	rec = call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": uuidString(b.ID), "value": "interested", "source": "mars"}, a.TelegramUserID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad source: status = %d, want 422", rec.Code)
	}
	rec = call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": "not-a-uuid", "value": "interested"}, a.TelegramUserID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad uuid: status = %d, want 422", rec.Code)
	}
	// source опционален (default deck)
	rec = call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": uuidString(b.ID), "value": "not_interested"}, a.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Errorf("no source: status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var src string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT source FROM evaluation WHERE evaluator_id = $1 AND target_id = $2", a.ID, b.ID).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if src != "deck" {
		t.Errorf("default source = %q, want deck", src)
	}
}

// FR-7: ответ API не раскрывает факт встречной оценки и контакт до взаимности.
func TestEvaluationResponseHidesIncomingInterest(t *testing.T) {
	e := newTestEnv(t)
	a := mkProfile(t, e, nil)
	b := mkProfile(t, e, nil)
	evaluate(t, e, a, b.ID, "interested")

	// B оценивает A «не интересно»: входящее «интересно» A не раскрывается
	rec := evaluate(t, e, b, a.ID, "not_interested")
	if rec.Code != http.StatusCreated {
		t.Fatalf("B->A: %d", rec.Code)
	}
	body := rec.Body.String()
	for _, leak := range []string{`"card"`, `"first_name"`, `"telegram_user_id"`, `"telegram_username"`} {
		if strings.Contains(body, leak) {
			t.Errorf("response leaks %s before reciprocity: %s", leak, body)
		}
	}
}
