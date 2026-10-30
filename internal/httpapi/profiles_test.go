package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/clubmatch/clubmatch/internal/auth"
	"github.com/clubmatch/clubmatch/internal/profile"
	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var httpTgSeq = time.Now().UnixNano() % 1_000_000_000

func nextTG() int64 {
	httpTgSeq++
	return 6_000_000_000 + httpTgSeq
}

var runID = fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)

type testEnv struct {
	pool *pgxpool.Pool
	st   *store.Store
	h    *ProfileHandler
	ah   *ApplicationHandler
	mh   *ModerationHandler
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	st := store.NewStore(pool)
	return &testEnv{
		pool: pool,
		st:   st,
		h:    NewProfileHandler(pool, st),
		ah:   NewApplicationHandler(pool, st),
		mh:   NewModerationHandler(pool, st, "testbot", "testapp"),
	}
}

func (e *testEnv) createUser(t *testing.T, accessState string) store.User {
	t.Helper()
	tg := nextTG()
	u, err := e.st.UpsertUserByTelegramID(context.Background(), tg, nil)
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	if accessState != "none" {
		if _, err := e.pool.Exec(context.Background(),
			"UPDATE users SET access_state = $2 WHERE id = $1", u.ID, accessState); err != nil {
			t.Fatalf("set access_state: %v", err)
		}
		u.AccessState = store.UserAccessState(accessState)
	}
	return u
}

func call(h http.HandlerFunc, method, target string, body any, tgID int64) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			panic(err)
		}
	}
	req := httptest.NewRequest(method, target, &buf)
	req = req.WithContext(auth.NewContext(req.Context(), auth.Identity{TelegramUserID: tgID}))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func validProfileBody() map[string]any {
	return map[string]any{
		"first_name":      "Иван",
		"skills":          []string{"Go Autotest" + runID},
		"meeting_format":  "online",
		"consent_version": "v1",
	}
}

func decodeSelf(t *testing.T, rec *httptest.ResponseRecorder) profileSelfDTO {
	t.Helper()
	var out profileSelfDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return out
}

func TestCreateProfileWithInvite(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.createUser(t, "none")
	token := "tok-" + runID + "-" + fmt.Sprint(u.TelegramUserID)
	if _, err := e.st.CreateInvite(ctx, store.CreateInviteParams{Token: token, CreatedBy: u.ID}); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	loc := "  Москва  "
	skill := "Kafka Autotest" + runID
	body := validProfileBody()
	body["invite_token"] = token
	body["skills"] = []string{skill}
	body["location"] = loc
	body["projects"] = []string{"Миграция на микросервисы"}
	body["professional_interests"] = []string{"Базы данных"}
	body["personal_interests"] = []string{"Шахматы"}
	body["last_name"] = "Петров"

	rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	out := decodeSelf(t, rec)
	if out.FirstName != "Иван" || out.LastName != "Петров" {
		t.Errorf("name = %q %q", out.FirstName, out.LastName)
	}
	if len(out.Skills) != 1 || out.Skills[0] != skill {
		t.Errorf("skills = %v", out.Skills)
	}
	if out.Location != "Москва" {
		t.Errorf("location = %q", out.Location)
	}
	if out.BotLinked {
		t.Error("bot_linked must be false without chat_id")
	}
	if !out.IsSearchable || out.UpdatedAt == "" {
		t.Errorf("is_searchable=%v updated_at=%q", out.IsSearchable, out.UpdatedAt)
	}

	// user became onboarded member
	got, err := e.st.GetUserByTelegramID(ctx, u.TelegramUserID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessState != "onboarded" || got.Role != "member" {
		t.Errorf("access_state=%s role=%s", got.AccessState, got.Role)
	}
	// invite consumed
	inv, err := e.st.GetInviteByToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if !inv.UsedBy.Valid || inv.UsedBy.Bytes != u.ID.Bytes {
		t.Errorf("invite not consumed: %+v", inv)
	}
	// consent recorded
	var consents int
	if err := e.pool.QueryRow(ctx,
		"SELECT count(*) FROM consent WHERE user_id = $1 AND document_version = 'v1'", u.ID).Scan(&consents); err != nil {
		t.Fatal(err)
	}
	if consents != 1 {
		t.Errorf("consent rows = %d", consents)
	}
	// location normalized
	var locNorm *string
	if err := e.pool.QueryRow(ctx,
		"SELECT location_norm FROM profile WHERE user_id = $1", u.ID).Scan(&locNorm); err != nil {
		t.Fatal(err)
	}
	if locNorm == nil || *locNorm != "москва" {
		t.Errorf("location_norm = %v", locNorm)
	}
	// fts_doc built with skill at weight A
	var fts string
	if err := e.pool.QueryRow(ctx,
		"SELECT fts_doc::text FROM profile WHERE user_id = $1", u.ID).Scan(&fts); err != nil {
		t.Fatal(err)
	}
	if fts == "" {
		t.Error("fts_doc is empty")
	}
	var ftsMatch bool
	if err := e.pool.QueryRow(ctx,
		"SELECT fts_doc @@ websearch_to_tsquery('russian', 'kafka') FROM profile WHERE user_id = $1", u.ID).Scan(&ftsMatch); err != nil {
		t.Fatal(err)
	}
	if !ftsMatch {
		t.Errorf("fts_doc does not match 'kafka': %q", fts)
	}
}

func TestCreateProfileInviteOneShot(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	owner := e.createUser(t, "none")
	token := "tok-" + runID + "-oneshot"
	if _, err := e.st.CreateInvite(ctx, store.CreateInviteParams{Token: token, CreatedBy: owner.ID}); err != nil {
		t.Fatal(err)
	}
	u1 := e.createUser(t, "none")
	body := validProfileBody()
	body["invite_token"] = token
	rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u1.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first use: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	u2 := e.createUser(t, "none")
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u2.TelegramUserID)
	if rec.Code != http.StatusForbidden {
		t.Errorf("second use of invite: status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	// unknown token
	u3 := e.createUser(t, "none")
	body["invite_token"] = "tok-" + runID + "-missing"
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u3.TelegramUserID)
	if rec.Code != http.StatusForbidden {
		t.Errorf("unknown invite: status = %d, want 403", rec.Code)
	}
}

func TestCreateProfileAccessAndConsent(t *testing.T) {
	e := newTestEnv(t)
	// no access, no token
	u := e.createUser(t, "none")
	rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", validProfileBody(), u.TelegramUserID)
	if rec.Code != http.StatusForbidden {
		t.Errorf("no access: status = %d, want 403", rec.Code)
	}
	// approved but no consent
	u = e.createUser(t, "approved")
	body := validProfileBody()
	delete(body, "consent_version")
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u.TelegramUserID)
	if rec.Code != http.StatusForbidden {
		t.Errorf("no consent: status = %d, want 403", rec.Code)
	}
	// approved + consent -> 201
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", validProfileBody(), u.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Errorf("approved onboarding: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	// invited state also allowed
	u = e.createUser(t, "invited")
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", validProfileBody(), u.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Errorf("invited onboarding: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateProfileValidation422(t *testing.T) {
	e := newTestEnv(t)
	cases := []struct {
		name   string
		mutate func(map[string]any)
		field  string
	}{
		{"missing first_name", func(b map[string]any) { delete(b, "first_name") }, "first_name"},
		{"short first_name", func(b map[string]any) { b["first_name"] = "Я" }, "first_name"},
		{"long first_name", func(b map[string]any) { b["first_name"] = string(make([]byte, 65)) }, "first_name"},
		{"no skills", func(b map[string]any) { b["skills"] = []string{} }, "skills"},
		{"duplicate skills", func(b map[string]any) { b["skills"] = []string{"Kafka", " kafka "} }, "skills"},
		{"bad meeting_format", func(b map[string]any) { b["meeting_format"] = "hybrid" }, "meeting_format"},
		{"missing meeting_format", func(b map[string]any) { delete(b, "meeting_format") }, "meeting_format"},
		{"long project", func(b map[string]any) { b["projects"] = []string{string(make([]byte, 257))} }, "projects"},
		{"short location", func(b map[string]any) { b["location"] = "М" }, "location"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := e.createUser(t, "approved")
			body := validProfileBody()
			tc.mutate(body)
			rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u.TelegramUserID)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
			}
			var errBody errorDTO
			if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			found := false
			for _, f := range errBody.Fields {
				if f == tc.field {
					found = true
				}
			}
			if !found {
				t.Errorf("expected field %q in %v", tc.field, errBody.Fields)
			}
		})
	}
}

func skillTagID(t *testing.T, e *testEnv, userID pgtype.UUID) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := e.pool.QueryRow(context.Background(),
		"SELECT tag_id FROM profile_tag WHERE profile_id = $1 AND kind = 'skill'", userID).Scan(&id); err != nil {
		t.Fatalf("skill tag id: %v", err)
	}
	return id
}

func TestTagDedup(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	base := "Dedup" + runID

	mk := func(skill string) store.User {
		u := e.createUser(t, "approved")
		body := validProfileBody()
		body["skills"] = []string{skill}
		rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u.TelegramUserID)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create with skill %q: status = %d, body = %s", skill, rec.Code, rec.Body.String())
		}
		return u
	}

	// «Kafka …» creates the tag
	u1 := mk("Kafka " + base)
	tagID := skillTagID(t, e, u1.ID)

	// «kafka …» (другой регистр) — тот же тег по normalized
	u2 := mk("kafka " + base)
	if got := skillTagID(t, e, u2.ID); got.Bytes != tagID.Bytes {
		t.Errorf("'kafka' resolved to another tag: %s != %s", got.Bytes, tagID.Bytes)
	}

	// Регистрируем алиас «Кафка …» (кросс-скриптовая связка — через справочник алиасов, AC-5)
	aliasRaw := "Кафка " + base
	aliasNorm := profile.Normalize(aliasRaw)
	lexemes, err := e.st.LexemesOfText(ctx, aliasNorm)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateTagAlias(ctx, store.CreateTagAliasParams{
		TagID: tagID, Alias: aliasRaw, Normalized: aliasNorm, Lexemes: lexemes,
	}); err != nil {
		t.Fatal(err)
	}

	// «Кафка …» — тот же тег по алиасу (normalized)
	u3 := mk(aliasRaw)
	if got := skillTagID(t, e, u3.ID); got.Bytes != tagID.Bytes {
		t.Errorf("'Кафка' resolved to another tag: %s != %s", got.Bytes, tagID.Bytes)
	}

	// «кафкой …» — тот же тег по lexemes (стемминг)
	u4 := mk("кафкой " + base)
	if got := skillTagID(t, e, u4.ID); got.Bytes != tagID.Bytes {
		t.Errorf("'кафкой' resolved to another tag: %s != %s", got.Bytes, tagID.Bytes)
	}

	// всего один тег в этой семье
	var count int
	if err := e.pool.QueryRow(ctx,
		"SELECT count(*) FROM tag WHERE normalized LIKE '%' || $1", profile.Normalize(base)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("tags in family = %d, want 1", count)
	}
}

func TestGetMe(t *testing.T) {
	e := newTestEnv(t)
	u := e.createUser(t, "approved")

	// no profile yet
	rec := call(e.h.GetMe, http.MethodGet, "/api/v1/profiles/me", nil, u.TelegramUserID)
	if rec.Code != http.StatusNotFound {
		t.Errorf("no profile: status = %d, want 404", rec.Code)
	}

	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", validProfileBody(), u.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	rec = call(e.h.GetMe, http.MethodGet, "/api/v1/profiles/me", nil, u.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("get me: %d %s", rec.Code, rec.Body.String())
	}
	out := decodeSelf(t, rec)
	if out.FirstName != "Иван" || out.BotLinked {
		t.Errorf("self = %+v", out)
	}

	// bot linked
	if err := e.st.SetUserChatID(context.Background(), u.TelegramUserID, 777); err != nil {
		t.Fatal(err)
	}
	rec = call(e.h.GetMe, http.MethodGet, "/api/v1/profiles/me", nil, u.TelegramUserID)
	out = decodeSelf(t, rec)
	if !out.BotLinked {
		t.Error("bot_linked must be true after chat_id set")
	}
}

func TestPatchMe(t *testing.T) {
	e := newTestEnv(t)
	u := e.createUser(t, "approved")
	rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", validProfileBody(), u.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	// patch first_name + skills
	newSkill := "PostgreSQL Autotest" + runID
	rec = call(e.h.PatchMe, http.MethodPatch, "/api/v1/profiles/me",
		map[string]any{"first_name": "Анна", "skills": []string{newSkill}}, u.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	out := decodeSelf(t, rec)
	if out.FirstName != "Анна" {
		t.Errorf("first_name = %q", out.FirstName)
	}
	if len(out.Skills) != 1 || out.Skills[0] != newSkill {
		t.Errorf("skills = %v, want [%s]", out.Skills, newSkill)
	}
	// fts rebuilt
	var ftsMatch bool
	if err := e.pool.QueryRow(context.Background(),
		"SELECT fts_doc @@ websearch_to_tsquery('russian', 'postgresql') FROM profile WHERE user_id = $1", u.ID).Scan(&ftsMatch); err != nil {
		t.Fatal(err)
	}
	if !ftsMatch {
		t.Error("fts_doc not rebuilt after patch")
	}

	// invariant: empty first_name rejected
	rec = call(e.h.PatchMe, http.MethodPatch, "/api/v1/profiles/me",
		map[string]any{"first_name": ""}, u.TelegramUserID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("empty first_name: status = %d, want 422", rec.Code)
	}
	// invariant: empty skills rejected
	rec = call(e.h.PatchMe, http.MethodPatch, "/api/v1/profiles/me",
		map[string]any{"skills": []string{}}, u.TelegramUserID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("empty skills: status = %d, want 422", rec.Code)
	}
	// bad enum rejected
	rec = call(e.h.PatchMe, http.MethodPatch, "/api/v1/profiles/me",
		map[string]any{"meeting_format": "hybrid"}, u.TelegramUserID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad meeting_format: status = %d, want 422", rec.Code)
	}

	// set then clear location
	rec = call(e.h.PatchMe, http.MethodPatch, "/api/v1/profiles/me",
		map[string]any{"location": "Санкт-Петербург"}, u.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch location: %d", rec.Code)
	}
	if out := decodeSelf(t, rec); out.Location != "Санкт-Петербург" {
		t.Errorf("location = %q", out.Location)
	}
	var locNorm *string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT location_norm FROM profile WHERE user_id = $1", u.ID).Scan(&locNorm); err != nil {
		t.Fatal(err)
	}
	if locNorm == nil || *locNorm != "санкт-петербург" {
		t.Errorf("location_norm = %v", locNorm)
	}
	rec = call(e.h.PatchMe, http.MethodPatch, "/api/v1/profiles/me",
		map[string]any{"location": ""}, u.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear location: %d %s", rec.Code, rec.Body.String())
	}
	if out := decodeSelf(t, rec); out.Location != "" {
		t.Errorf("location after clear = %q", out.Location)
	}

	// patch without profile -> 404
	u2 := e.createUser(t, "approved")
	rec = call(e.h.PatchMe, http.MethodPatch, "/api/v1/profiles/me",
		map[string]any{"first_name": "Анна"}, u2.TelegramUserID)
	if rec.Code != http.StatusNotFound {
		t.Errorf("patch without profile: status = %d, want 404", rec.Code)
	}
}

func TestAutocomplete(t *testing.T) {
	e := newTestEnv(t)
	u := e.createUser(t, "approved")
	base := "Autocomp" + runID
	body := validProfileBody()
	body["skills"] = []string{"Kafka " + base, "Kotlin " + base}
	rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, u.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	// prefix by canonical normalized
	contains := func(tags []tagDTO, canonical string) bool {
		for _, tg := range tags {
			if tg.Canonical == canonical {
				if tg.ID == "" {
					t.Error("tag id empty")
				}
				return true
			}
		}
		return false
	}
	rec = call(e.h.Autocomplete, http.MethodGet, "/api/v1/tags/autocomplete?q="+url.QueryEscape("kafka "+base), nil, u.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("autocomplete: %d %s", rec.Code, rec.Body.String())
	}
	var tags []tagDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatal(err)
	}
	if !contains(tags, "Kafka "+base) {
		t.Errorf("q='kafka %s' -> %+v", base, tags)
	}

	// case-insensitive: KAFKA -> kafka
	rec = call(e.h.Autocomplete, http.MethodGet, "/api/v1/tags/autocomplete?q="+url.QueryEscape(strings.ToUpper("kafka "+base)), nil, u.TelegramUserID)
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatal(err)
	}
	if !contains(tags, "Kafka "+base) {
		t.Errorf("q=KAFKA %s -> %+v", base, tags)
	}

	// match by alias normalized
	ctx := context.Background()
	var tagID pgtype.UUID
	if err := e.pool.QueryRow(ctx,
		`SELECT pt.tag_id FROM profile_tag pt JOIN tag t ON t.id = pt.tag_id
		 WHERE pt.profile_id = $1 AND pt.kind = 'skill' AND t.canonical = $2`,
		u.ID, "Kafka "+base).Scan(&tagID); err != nil {
		t.Fatalf("kafka tag id: %v", err)
	}
	aliasNorm := profile.Normalize("Кафка " + base)
	lex, err := e.st.LexemesOfText(ctx, aliasNorm)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateTagAlias(ctx, store.CreateTagAliasParams{
		TagID: tagID, Alias: "Кафка " + base, Normalized: aliasNorm, Lexemes: lex,
	}); err != nil {
		t.Fatal(err)
	}
	rec = call(e.h.Autocomplete, http.MethodGet, "/api/v1/tags/autocomplete?q="+url.QueryEscape("кафка "+base), nil, u.TelegramUserID)
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatal(err)
	}
	if !contains(tags, "Kafka "+base) {
		t.Errorf("q='кафка %s' -> %+v, want Kafka tag via alias", base, tags)
	}

	// q bounds
	rec = call(e.h.Autocomplete, http.MethodGet, "/api/v1/tags/autocomplete?q=", nil, u.TelegramUserID)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty q: status = %d, want 400", rec.Code)
	}
	long := strings.Repeat("a", 65)
	rec = call(e.h.Autocomplete, http.MethodGet, "/api/v1/tags/autocomplete?q="+long, nil, u.TelegramUserID)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("long q: status = %d, want 400", rec.Code)
	}
}
