package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// mkProfile creates a user with a completed profile; extra maps additional
// fields of the profile body (skills/interests/projects).
func mkProfile(t *testing.T, e *testEnv, extra map[string]any) store.User {
	t.Helper()
	u := e.createUser(t, "approved")
	body := validProfileBody()
	for k, v := range extra {
		body[k] = v
	}
	rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create profile: %d %s", rec.Code, rec.Body.String())
	}
	return u
}

func deckIDs(t *testing.T, e *testEnv, viewer pgtype.UUID, limit int32) []string {
	t.Helper()
	rows, err := e.st.ListDeckCards(context.Background(), store.ListDeckCardsParams{
		ProfileID: viewer,
		Limit:     limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, uuidString(r.UserID))
	}
	return ids
}

func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

func TestDeckExclusions(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tag := "DeckExcl" + runID
	viewer := mkProfile(t, e, map[string]any{"skills": []string{tag + " viewer"}})

	// оценённая < 30 дней назад — исключена (FR-9)
	recent := mkProfile(t, e, map[string]any{"skills": []string{tag + " recent"}})
	rec := call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": uuidString(recent.ID), "value": "not_interested"}, viewer.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("evaluate: %d %s", rec.Code, rec.Body.String())
	}

	// оценённая > 30 дней назад — снова допущена (FR-9, AC-3)
	old := mkProfile(t, e, map[string]any{"skills": []string{tag + " old"}})
	rec = call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": uuidString(old.ID), "value": "not_interested"}, viewer.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("evaluate old: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := e.pool.Exec(ctx,
		"UPDATE evaluation SET updated_at = now() - interval '31 days' WHERE evaluator_id = $1 AND target_id = $2",
		viewer.ID, old.ID); err != nil {
		t.Fatal(err)
	}

	// is_searchable=false — исключена (FR-4)
	hidden := mkProfile(t, e, map[string]any{"skills": []string{tag + " hidden"}})
	if _, err := e.pool.Exec(ctx, "UPDATE profile SET is_searchable = false WHERE user_id = $1", hidden.ID); err != nil {
		t.Fatal(err)
	}

	// удалённый пользователь — исключён
	deleted := mkProfile(t, e, map[string]any{"skills": []string{tag + " deleted"}})
	if _, err := e.pool.Exec(ctx, "UPDATE users SET deleted_at = now() WHERE id = $1", deleted.ID); err != nil {
		t.Fatal(err)
	}

	plain := mkProfile(t, e, map[string]any{"skills": []string{tag + " plain"}})

	ids := deckIDs(t, e, viewer.ID, 1000)
	for name, u := range map[string]store.User{
		"self": viewer, "recent": recent, "hidden": hidden, "deleted": deleted,
	} {
		if indexOf(ids, uuidString(u.ID)) >= 0 {
			t.Errorf("%s must be excluded from deck", name)
		}
	}
	for name, u := range map[string]store.User{"old": old, "plain": plain} {
		if indexOf(ids, uuidString(u.ID)) < 0 {
			t.Errorf("%s must be present in deck", name)
		}
	}
}

func TestDeckGrouping(t *testing.T) {
	e := newTestEnv(t)
	skillTag := "DeckGrpSkill" + runID
	persTag := "DeckGrpPers" + runID
	viewer := mkProfile(t, e, map[string]any{
		"skills":             []string{skillTag},
		"personal_interests": []string{persTag},
	})
	bySkill := mkProfile(t, e, map[string]any{"skills": []string{skillTag}})
	byProf := mkProfile(t, e, map[string]any{"professional_interests": []string{skillTag}})
	byPersonal := mkProfile(t, e, map[string]any{"personal_interests": []string{persTag}})
	noMatch := mkProfile(t, e, map[string]any{"skills": []string{"DeckGrpNone" + runID}})

	ids := deckIDs(t, e, viewer.ID, 1000)
	pos := func(u store.User) int {
		i := indexOf(ids, uuidString(u.ID))
		if i < 0 {
			t.Fatalf("card %s not in deck", uuidString(u.ID))
		}
		return i
	}
	// совпадение по навыку/проф. интересу — раньше совпадения только по личному (AC-3)
	if pos(bySkill) > pos(byPersonal) || pos(byProf) > pos(byPersonal) {
		t.Errorf("skill/prof matches must precede personal-only match: %v", ids)
	}
	// любое совпадение — раньше карточек без совпадения (FR-5)
	if pos(byPersonal) > pos(noMatch) {
		t.Errorf("personal match must precede no-match card: %v", ids)
	}
	if pos(bySkill) > pos(noMatch) || pos(byProf) > pos(noMatch) {
		t.Errorf("skill/prof match must precede no-match card: %v", ids)
	}
}

// AC-7/NFR-8: до матча ответ колоды не содержит скрытых полей (проверка сырого ответа).
func TestDeckContractPreMatch(t *testing.T) {
	e := newTestEnv(t)
	tag := "DeckAC7" + runID
	viewer := mkProfile(t, e, map[string]any{"skills": []string{tag}})
	mkProfile(t, e, map[string]any{"skills": []string{tag}})

	rec := call(e.dh.Get, http.MethodGet, "/api/v1/deck", nil, viewer.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("deck: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, hidden := range []string{`"first_name"`, `"last_name"`, `"avatar_url"`, `"telegram_username"`, `"telegram_user_id"`} {
		if strings.Contains(body, hidden) {
			t.Errorf("pre_match deck response must not contain %s; body=%s", hidden, body)
		}
	}
	var cards []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cards); err != nil {
		t.Fatalf("deck response is not a JSON array: %v", err)
	}
	if len(cards) > deckLimit {
		t.Errorf("deck size = %d, limit = %d", len(cards), deckLimit)
	}
	for _, c := range cards {
		for _, want := range []string{"id", "skills", "projects", "professional_interests", "personal_interests", "meeting_format"} {
			if _, ok := c[want]; !ok {
				t.Errorf("card missing %q: %v", want, c)
			}
		}
	}
}

// UC-3 E1: исчерпанная колода — 200 и пустой массив, не ошибка.
func TestDeckEmpty(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	viewer := mkProfile(t, e, nil)
	// исключаем всех: оцениваем всех searchable-участников напрямую
	rows, err := e.pool.Query(ctx,
		"SELECT user_id FROM profile WHERE user_id <> $1 AND is_searchable", viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	var others []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		others = append(others, id)
	}
	rows.Close()
	for _, id := range others {
		if _, err := e.pool.Exec(ctx,
			"INSERT INTO evaluation (evaluator_id, target_id, value, source) VALUES ($1, $2, 'not_interested', 'deck')",
			viewer.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	rec := call(e.dh.Get, http.MethodGet, "/api/v1/deck", nil, viewer.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty deck: %d %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty deck body = %s, want []", rec.Body.String())
	}
}

func TestDeckLimit(t *testing.T) {
	e := newTestEnv(t)
	tag := "DeckLimit" + runID
	viewer := mkProfile(t, e, map[string]any{"skills": []string{tag}})
	for i := 0; i < deckLimit+3; i++ {
		mkProfile(t, e, map[string]any{"skills": []string{tag + fmt.Sprint(i)}})
	}
	rec := call(e.dh.Get, http.MethodGet, "/api/v1/deck", nil, viewer.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("deck: %d %s", rec.Code, rec.Body.String())
	}
	var cards []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cards); err != nil {
		t.Fatal(err)
	}
	if len(cards) != deckLimit {
		t.Errorf("deck size = %d, want %d", len(cards), deckLimit)
	}
}
