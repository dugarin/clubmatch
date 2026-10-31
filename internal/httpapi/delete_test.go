package httpapi

import (
	"context"
	"net/http"
	"testing"
)

func TestDeleteProfileFR17(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := mkProfile(t, e, map[string]any{"skills": []string{"DelA" + runID}})
	b := mkProfile(t, e, map[string]any{"skills": []string{"DelB" + runID}})
	makeMatch(t, e,
		&store_UserRef{id: a.ID, tgID: a.TelegramUserID},
		&store_UserRef{id: b.ID, tgID: b.TelegramUserID})

	rec := call(e.h.DeleteMe, http.MethodDelete, "/api/v1/profiles/me", nil, b.TelegramUserID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}

	for table, col := range map[string]string{
		"profile":            "user_id",
		"profile_tag":        "profile_id",
		"project":            "profile_id",
		"consent":            "user_id",
		"evaluation":         "target_id",
		"evaluation_history": "target_id",
	} {
		var n int
		if err := e.pool.QueryRow(ctx,
			"SELECT count(*) FROM "+table+" WHERE "+col+" = $1", b.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s still has rows for deleted user", table)
		}
	}

	var matchState string
	if err := e.pool.QueryRow(ctx,
		"SELECT state::text FROM match WHERE user_low = least($1::uuid,$2::uuid) AND user_high = greatest($1::uuid,$2::uuid)",
		a.ID, b.ID).Scan(&matchState); err != nil {
		t.Fatal(err)
	}
	if matchState != "contact_unavailable" {
		t.Errorf("match state = %s", matchState)
	}

	var journals int
	if err := e.pool.QueryRow(ctx,
		"SELECT count(*) FROM contact_disclosure_journal WHERE subject_id = $1 OR recipient_id = $1",
		b.ID).Scan(&journals); err != nil {
		t.Fatal(err)
	}
	if journals == 0 {
		t.Error("disclosure journal must survive profile deletion (NFR-10)")
	}

	var deletedAt bool
	if err := e.pool.QueryRow(ctx,
		"SELECT deleted_at IS NOT NULL FROM users WHERE id = $1", b.ID).Scan(&deletedAt); err != nil {
		t.Fatal(err)
	}
	if !deletedAt {
		t.Error("user must be soft-deleted")
	}

	items := listMatches(t, e, a.TelegramUserID)
	if len(items) != 1 || items[0].State != "contact_unavailable" || items[0].Card != nil {
		t.Errorf("second side must see contact_unavailable without card: %+v", items)
	}
}

func TestDeleteProfileNotFound(t *testing.T) {
	e := newTestEnv(t)
	u := e.createUser(t, "approved")
	rec := call(e.h.DeleteMe, http.MethodDelete, "/api/v1/profiles/me", nil, u.TelegramUserID)
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete without profile: %d", rec.Code)
	}
}
