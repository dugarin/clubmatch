package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/clubmatch/clubmatch/internal/auth"
	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (e *testEnv) createModerator(t *testing.T) store.User {
	t.Helper()
	u := e.createUser(t, "approved")
	if _, err := e.pool.Exec(context.Background(),
		"UPDATE users SET role = 'moderator' WHERE id = $1", u.ID); err != nil {
		t.Fatalf("set moderator role: %v", err)
	}
	u.Role = store.UserRoleModerator
	return u
}

func (e *testEnv) modMux() http.Handler {
	r := chi.NewRouter()
	r.Use(RequireModerator(e.st))
	r.Get("/api/v1/moderation/applications", e.mh.ListApplications)
	r.Post("/api/v1/moderation/applications/{id}/approve", e.mh.Approve)
	r.Post("/api/v1/moderation/applications/{id}/reject", e.mh.Reject)
	r.Post("/api/v1/moderation/invites", e.mh.CreateInvite)
	r.Post("/api/v1/moderation/tags/{id}/revoke", e.mh.RevokeTag)
	return r
}

func callH(h http.Handler, method, target string, body any, tgID int64) *httptest.ResponseRecorder {
	var buf strings.Builder
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			panic(err)
		}
	}
	req := httptest.NewRequest(method, target, strings.NewReader(buf.String()))
	req = req.WithContext(auth.NewContext(req.Context(), auth.Identity{TelegramUserID: tgID}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func pendingAppID(t *testing.T, e *testEnv, candidateID pgtype.UUID) string {
	t.Helper()
	a, err := e.st.GetPendingApplicationByCandidate(context.Background(), candidateID)
	if err != nil {
		t.Fatalf("pending application: %v", err)
	}
	return uuidString(a.ID)
}

func countNotifications(t *testing.T, e *testEnv, userID pgtype.UUID, ntype string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM notification WHERE user_id = $1 AND type = $2 AND state = 'pending'",
		userID, ntype).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return n
}

func countJournal(t *testing.T, e *testEnv, action, targetType, targetID string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM moderation_journal WHERE action = $1 AND target_type = $2 AND target_id = $3::uuid",
		action, targetType, targetID).Scan(&n); err != nil {
		t.Fatalf("count journal: %v", err)
	}
	return n
}

func accessState(t *testing.T, e *testEnv, userID pgtype.UUID) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT access_state FROM users WHERE id = $1", userID).Scan(&s); err != nil {
		t.Fatalf("access state: %v", err)
	}
	return s
}

func TestApplicationSubmitAndStatus(t *testing.T) {
	e := newTestEnv(t)
	u := e.createUser(t, "none")

	rec := call(e.ah.Submit, http.MethodPost, "/api/v1/applications", nil, u.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var dto applicationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.State != "pending" || dto.ID == "" || dto.CreatedAt == "" {
		t.Errorf("dto = %+v", dto)
	}
	if got := accessState(t, e, u.ID); got != "none" {
		t.Errorf("access_state = %s, want none (pending is not an access state)", got)
	}

	// active application already exists
	rec = call(e.ah.Submit, http.MethodPost, "/api/v1/applications", nil, u.TelegramUserID)
	if rec.Code != http.StatusConflict {
		t.Errorf("second submit: status = %d, want 409", rec.Code)
	}

	// status screen
	rec = call(e.ah.GetMe, http.MethodGet, "/api/v1/applications/me", nil, u.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var st applicationStatusDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Application == nil || st.Application.State != "pending" || st.AccessState != "none" {
		t.Errorf("status = %+v", st)
	}

	// no application yet
	fresh := e.createUser(t, "none")
	rec = call(e.ah.GetMe, http.MethodGet, "/api/v1/applications/me", nil, fresh.TelegramUserID)
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Application != nil {
		t.Errorf("fresh user application = %+v, want null", st.Application)
	}

	// already has access -> 403
	for _, state := range []string{"approved", "onboarded", "invited"} {
		m := e.createUser(t, state)
		rec = call(e.ah.Submit, http.MethodPost, "/api/v1/applications", nil, m.TelegramUserID)
		if rec.Code != http.StatusForbidden {
			t.Errorf("submit with state %s: status = %d, want 403", state, rec.Code)
		}
	}
}

func TestModerationGateForbidden(t *testing.T) {
	e := newTestEnv(t)
	mux := e.modMux()
	for _, role := range []string{"none", "approved"} {
		u := e.createUser(t, role)
		targets := []struct{ method, path string }{
			{http.MethodGet, "/api/v1/moderation/applications"},
			{http.MethodPost, "/api/v1/moderation/applications/" + uuidString(mkUUID(t)) + "/approve"},
			{http.MethodPost, "/api/v1/moderation/applications/" + uuidString(mkUUID(t)) + "/reject"},
			{http.MethodPost, "/api/v1/moderation/invites"},
			{http.MethodPost, "/api/v1/moderation/tags/" + uuidString(mkUUID(t)) + "/revoke"},
		}
		for _, tg := range targets {
			rec := callH(mux, tg.method, tg.path, nil, u.TelegramUserID)
			if rec.Code != http.StatusForbidden {
				t.Errorf("role state %s %s %s: status = %d, want 403", role, tg.method, tg.path, rec.Code)
			}
		}
	}
}

func mkUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan("019a0000-0000-7000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestModerationApproveFlow(t *testing.T) {
	e := newTestEnv(t)
	mux := e.modMux()
	mod := e.createModerator(t)
	cand := e.createUser(t, "none")

	rec := call(e.ah.Submit, http.MethodPost, "/api/v1/applications", nil, cand.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body.String())
	}
	appID := pendingAppID(t, e, cand.ID)

	// list pending
	rec = callH(mux, http.MethodGet, "/api/v1/moderation/applications?state=pending", nil, mod.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var list []moderationApplicationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range list {
		if item.ID == appID {
			found = true
			if item.TelegramUserID != cand.TelegramUserID {
				t.Errorf("telegram_user_id = %d, want %d", item.TelegramUserID, cand.TelegramUserID)
			}
		}
	}
	if !found {
		t.Errorf("application %s not in pending list: %+v", appID, list)
	}
	// bad state filter
	rec = callH(mux, http.MethodGet, "/api/v1/moderation/applications?state=bogus", nil, mod.TelegramUserID)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad state: status = %d, want 400", rec.Code)
	}

	// approve
	rec = callH(mux, http.MethodPost, "/api/v1/moderation/applications/"+appID+"/approve", nil, mod.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	if got := accessState(t, e, cand.ID); got != "approved" {
		t.Errorf("access_state = %s, want approved", got)
	}
	if n := countNotifications(t, e, cand.ID, "application_approved"); n != 1 {
		t.Errorf("application_approved notifications = %d, want 1", n)
	}
	if n := countJournal(t, e, "application_approved", "application", appID); n != 1 {
		t.Errorf("journal rows = %d, want 1", n)
	}

	// approved candidate can onboard without invite (UC-6)
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", validProfileBody(), cand.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Errorf("onboarding after approve: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// repeated approve -> 409
	rec = callH(mux, http.MethodPost, "/api/v1/moderation/applications/"+appID+"/approve", nil, mod.TelegramUserID)
	if rec.Code != http.StatusConflict {
		t.Errorf("repeat approve: status = %d, want 409", rec.Code)
	}
	// reject after approve -> 409
	rec = callH(mux, http.MethodPost, "/api/v1/moderation/applications/"+appID+"/reject", nil, mod.TelegramUserID)
	if rec.Code != http.StatusConflict {
		t.Errorf("reject after approve: status = %d, want 409", rec.Code)
	}
	// unknown id -> 404
	rec = callH(mux, http.MethodPost, "/api/v1/moderation/applications/"+uuidString(mkUUID(t))+"/approve", nil, mod.TelegramUserID)
	if rec.Code != http.StatusNotFound {
		t.Errorf("approve unknown: status = %d, want 404", rec.Code)
	}
}

func TestModerationRejectAndResubmit(t *testing.T) {
	e := newTestEnv(t)
	mux := e.modMux()
	mod := e.createModerator(t)
	cand := e.createUser(t, "none")

	rec := call(e.ah.Submit, http.MethodPost, "/api/v1/applications", nil, cand.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body.String())
	}
	appID := pendingAppID(t, e, cand.ID)

	rec = callH(mux, http.MethodPost, "/api/v1/moderation/applications/"+appID+"/reject", nil, mod.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rec.Code, rec.Body.String())
	}
	if got := accessState(t, e, cand.ID); got != "rejected" {
		t.Errorf("access_state = %s, want rejected", got)
	}
	if n := countNotifications(t, e, cand.ID, "application_rejected"); n != 1 {
		t.Errorf("application_rejected notifications = %d, want 1", n)
	}
	if n := countJournal(t, e, "application_rejected", "application", appID); n != 1 {
		t.Errorf("journal rows = %d, want 1", n)
	}
	// onboarding forbidden while rejected
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", validProfileBody(), cand.TelegramUserID)
	if rec.Code != http.StatusForbidden {
		t.Errorf("onboarding while rejected: status = %d, want 403", rec.Code)
	}

	// resubmit allowed after rejection (Q21)
	rec = call(e.ah.Submit, http.MethodPost, "/api/v1/applications", nil, cand.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("resubmit: %d %s", rec.Code, rec.Body.String())
	}
	// status screen shows the new pending application
	rec = call(e.ah.GetMe, http.MethodGet, "/api/v1/applications/me", nil, cand.TelegramUserID)
	var st applicationStatusDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Application == nil || st.Application.State != "pending" {
		t.Errorf("status after resubmit = %+v", st)
	}

	// approve the new application
	newID := pendingAppID(t, e, cand.ID)
	rec = callH(mux, http.MethodPost, "/api/v1/moderation/applications/"+newID+"/approve", nil, mod.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve resubmitted: %d %s", rec.Code, rec.Body.String())
	}
	if got := accessState(t, e, cand.ID); got != "approved" {
		t.Errorf("access_state = %s, want approved", got)
	}
	// old rejected application cannot be re-decided
	rec = callH(mux, http.MethodPost, "/api/v1/moderation/applications/"+appID+"/approve", nil, mod.TelegramUserID)
	if rec.Code != http.StatusConflict {
		t.Errorf("re-decide old: status = %d, want 409", rec.Code)
	}
}

func TestModerationInviteOneShot(t *testing.T) {
	e := newTestEnv(t)
	mux := e.modMux()
	mod := e.createModerator(t)

	rec := callH(mux, http.MethodPost, "/api/v1/moderation/invites", nil, mod.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(out.URL)
	if err != nil {
		t.Fatalf("parse url %q: %v", out.URL, err)
	}
	if u.Host != "t.me" || u.Path != "/testbot/testapp" {
		t.Errorf("url = %q, want https://t.me/testbot/testapp?startapp=<token>", out.URL)
	}
	token := u.Query().Get("startapp")
	if len(token) < 32 {
		t.Errorf("token too short: %q", token)
	}
	var inviteID string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT id FROM invite WHERE token = $1", token).Scan(&inviteID); err != nil {
		t.Fatalf("invite in db: %v", err)
	}
	if n := countJournal(t, e, "invite_created", "invite", inviteID); n != 1 {
		t.Errorf("journal rows = %d, want 1", n)
	}

	// one-shot: first onboarding consumes it, second is rejected (FR-13)
	body := validProfileBody()
	body["invite_token"] = token
	c1 := e.createUser(t, "none")
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, c1.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first use: %d %s", rec.Code, rec.Body.String())
	}
	c2 := e.createUser(t, "none")
	rec = call(e.h.Create, http.MethodPost, "/api/v1/profiles", body, c2.TelegramUserID)
	if rec.Code != http.StatusForbidden {
		t.Errorf("second use: status = %d, want 403", rec.Code)
	}
}

func TestModerationRevokeTag(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	mux := e.modMux()
	mod := e.createModerator(t)
	base := "Revoke" + runID
	soloSkill := "SoloSkill " + base
	otherSkill := "OtherSkill " + base

	// A: author of the tag, it is her only skill
	a := e.createUser(t, "approved")
	bodyA := validProfileBody()
	bodyA["skills"] = []string{soloSkill}
	if rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", bodyA, a.TelegramUserID); rec.Code != http.StatusCreated {
		t.Fatalf("create A: %s", rec.Body.String())
	}
	// B: has the revoked skill plus another one
	b := e.createUser(t, "approved")
	bodyB := validProfileBody()
	bodyB["skills"] = []string{soloSkill, otherSkill}
	if rec := call(e.h.Create, http.MethodPost, "/api/v1/profiles", bodyB, b.TelegramUserID); rec.Code != http.StatusCreated {
		t.Fatalf("create B: %s", rec.Body.String())
	}
	tagID := skillTagID(t, e, a.ID)
	tagIDStr := uuidString(tagID)

	rec := callH(mux, http.MethodPost, "/api/v1/moderation/tags/"+tagIDStr+"/revoke", nil, mod.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}

	// tag revoked in the directory
	var status string
	if err := e.pool.QueryRow(ctx, "SELECT status FROM tag WHERE id = $1", tagID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "revoked" {
		t.Errorf("tag status = %s, want revoked", status)
	}
	// cascaded out of all profiles
	var links int
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM profile_tag WHERE tag_id = $1", tagID).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Errorf("profile_tag rows = %d, want 0", links)
	}
	// A lost the last skill -> not searchable; B still searchable
	var searchableA, searchableB bool
	if err := e.pool.QueryRow(ctx, "SELECT is_searchable FROM profile WHERE user_id = $1", a.ID).Scan(&searchableA); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, "SELECT is_searchable FROM profile WHERE user_id = $1", b.ID).Scan(&searchableB); err != nil {
		t.Fatal(err)
	}
	if searchableA {
		t.Error("A.is_searchable = true, want false")
	}
	if !searchableB {
		t.Error("B.is_searchable = false, want true")
	}
	// notifications: tag author + last-skill loser
	if n := countNotifications(t, e, a.ID, "tag_revoked"); n != 1 {
		t.Errorf("tag_revoked for A = %d, want 1", n)
	}
	if n := countNotifications(t, e, a.ID, "last_skill_lost"); n != 1 {
		t.Errorf("last_skill_lost for A = %d, want 1", n)
	}
	if n := countNotifications(t, e, b.ID, "last_skill_lost"); n != 0 {
		t.Errorf("last_skill_lost for B = %d, want 0", n)
	}
	if n := countNotifications(t, e, b.ID, "tag_revoked"); n != 0 {
		t.Errorf("tag_revoked for B = %d, want 0 (B is not the author)", n)
	}
	// journal
	if n := countJournal(t, e, "tag_revoked", "tag", tagIDStr); n != 1 {
		t.Errorf("journal rows = %d, want 1", n)
	}
	// fts rebuilt: revoked skill gone from B's document
	var ftsMatch bool
	if err := e.pool.QueryRow(ctx,
		"SELECT fts_doc @@ websearch_to_tsquery('russian', 'soloskill') FROM profile WHERE user_id = $1",
		b.ID).Scan(&ftsMatch); err != nil {
		t.Fatal(err)
	}
	if ftsMatch {
		t.Error("B fts_doc still matches revoked skill")
	}
	// A's self view has no skills anymore (active-only listing)
	rec = call(e.h.GetMe, http.MethodGet, "/api/v1/profiles/me", nil, a.TelegramUserID)
	self := decodeSelf(t, rec)
	if len(self.Skills) != 0 {
		t.Errorf("A skills = %v, want empty", self.Skills)
	}
	if self.IsSearchable {
		t.Error("A self is_searchable = true, want false")
	}

	// repeated revoke -> 409
	rec = callH(mux, http.MethodPost, "/api/v1/moderation/tags/"+tagIDStr+"/revoke", nil, mod.TelegramUserID)
	if rec.Code != http.StatusConflict {
		t.Errorf("repeat revoke: status = %d, want 409", rec.Code)
	}
	// unknown tag -> 404
	rec = callH(mux, http.MethodPost, "/api/v1/moderation/tags/"+uuidString(mkUUID(t))+"/revoke", nil, mod.TelegramUserID)
	if rec.Code != http.StatusNotFound {
		t.Errorf("revoke unknown: status = %d, want 404", rec.Code)
	}
	// restored after adding a new skill (FR-4: карточка возвращается)
	rec = call(e.h.PatchMe, http.MethodPatch, "/api/v1/profiles/me",
		map[string]any{"skills": []string{"NewSkill " + base}}, a.TelegramUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch A: %d %s", rec.Code, rec.Body.String())
	}
	var restored bool
	if err := e.pool.QueryRow(ctx, "SELECT is_searchable FROM profile WHERE user_id = $1", a.ID).Scan(&restored); err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Error("A.is_searchable not restored after adding a new skill")
	}
}
