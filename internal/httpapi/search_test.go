package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

type searchTestResponse struct {
	Items          []map[string]any `json:"items"`
	Page           int              `json:"page"`
	HasMore        bool             `json:"has_more"`
	FiltersApplied bool             `json:"filters_applied"`
}

func doSearch(t *testing.T, e *testEnv, tgID int64, query string) (int, searchTestResponse) {
	t.Helper()
	rec := call(e.sh.Get, http.MethodGet, "/api/v1/search?"+query, nil, tgID)
	var out searchTestResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode search: %v; body=%s", err, rec.Body.String())
		}
	}
	return rec.Code, out
}

func TestSearchTagBeatsText(t *testing.T) {
	e := newTestEnv(t)
	tag := "KafkaSearch" + runID

	tagged := mkProfile(t, e, map[string]any{"skills": []string{tag}})
	textOnly := mkProfile(t, e, map[string]any{
		"skills":   []string{"GoSearch" + runID},
		"projects": []string{"Миграция на " + tag},
	})
	viewer := mkProfile(t, e, map[string]any{"skills": []string{"ViewerSearch" + runID}})

	code, res := doSearch(t, e, viewer.TelegramUserID, "q="+tag)
	if code != http.StatusOK {
		t.Fatalf("search: %d", code)
	}
	if len(res.Items) < 2 {
		t.Fatalf("expected both profiles, got %d", len(res.Items))
	}
	if res.Items[0]["id"] != uuidString(tagged.ID) {
		t.Errorf("tag match must outrank text match: first=%v want %v", res.Items[0]["id"], uuidString(tagged.ID))
	}
	if res.Items[1]["id"] != uuidString(textOnly.ID) {
		t.Errorf("text match second: %v", res.Items[1]["id"])
	}
}

func TestSearchFiltersAndSelfExclusion(t *testing.T) {
	e := newTestEnv(t)
	tag := "FilterSearch" + runID
	norm := "filtertown" + runID

	berlin := mkProfile(t, e, map[string]any{"skills": []string{tag}, "location": "FilterTown" + runID, "meeting_format": "offline"})
	mkProfile(t, e, map[string]any{"skills": []string{tag}, "location": "Elsewhere" + runID, "meeting_format": "offline"})
	anyFormat := mkProfile(t, e, map[string]any{"skills": []string{tag}, "location": "FilterTown" + runID, "meeting_format": "any"})
	onlineOnly := mkProfile(t, e, map[string]any{"skills": []string{tag}, "location": "FilterTown" + runID, "meeting_format": "online"})
	viewer := mkProfile(t, e, map[string]any{"skills": []string{tag}, "location": "FilterTown" + runID, "meeting_format": "online"})

	code, res := doSearch(t, e, viewer.TelegramUserID, "q="+tag+"&location="+norm+"&meeting_format=offline")
	if code != http.StatusOK {
		t.Fatalf("search: %d", code)
	}
	if !res.FiltersApplied {
		t.Error("filters_applied must be true")
	}
	ids := map[string]bool{}
	for _, it := range res.Items {
		ids[it["id"].(string)] = true
	}
	if !ids[uuidString(berlin.ID)] || !ids[uuidString(anyFormat.ID)] {
		t.Errorf("expected berlin and anyFormat in results: %v", ids)
	}
	if ids[uuidString(onlineOnly.ID)] {
		t.Error("online-only profile must not match offline filter")
	}
	if ids[uuidString(viewer.ID)] {
		t.Error("own profile must be excluded (FR-10)")
	}
	for _, it := range res.Items {
		if it["first_name"] != nil || it["telegram_username"] != nil {
			t.Errorf("pre_match search card leaks hidden fields: %v", it)
		}
	}
}

func TestSearchEmptyAndMyEvaluation(t *testing.T) {
	e := newTestEnv(t)
	tag := "EmptySearch" + runID
	other := mkProfile(t, e, map[string]any{"skills": []string{tag}})
	viewer := mkProfile(t, e, map[string]any{"skills": []string{"EmptyViewer" + runID}})

	code, res := doSearch(t, e, viewer.TelegramUserID, "q=no-such-term-"+runID)
	if code != http.StatusOK || len(res.Items) != 0 {
		t.Fatalf("empty result: code=%d items=%d", code, len(res.Items))
	}

	rec := call(e.eh.Create, http.MethodPost, "/api/v1/evaluations",
		map[string]any{"target_id": uuidString(other.ID), "value": "interested", "source": "search"}, viewer.TelegramUserID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("evaluate: %d %s", rec.Code, rec.Body.String())
	}
	_, res = doSearch(t, e, viewer.TelegramUserID, "q="+tag)
	if len(res.Items) != 1 || res.Items[0]["my_evaluation"] != "interested" {
		t.Errorf("my_evaluation label: %+v", res.Items)
	}
}

func TestSearchPagination(t *testing.T) {
	e := newTestEnv(t)
	tag := "PageSearch" + runID
	viewer := mkProfile(t, e, map[string]any{"skills": []string{"PageViewer" + runID}})
	for range 12 {
		mkProfile(t, e, map[string]any{"skills": []string{tag}})
	}
	_, p1 := doSearch(t, e, viewer.TelegramUserID, "q="+tag+"&page=1")
	if len(p1.Items) != 10 || !p1.HasMore {
		t.Fatalf("page1: items=%d has_more=%v", len(p1.Items), p1.HasMore)
	}
	_, p2 := doSearch(t, e, viewer.TelegramUserID, "q="+tag+"&page=2")
	if len(p2.Items) < 2 || p2.HasMore {
		t.Fatalf("page2: items=%d has_more=%v", len(p2.Items), p2.HasMore)
	}
	seen := map[string]bool{}
	for _, it := range append(p1.Items, p2.Items...) {
		id := it["id"].(string)
		if seen[id] {
			t.Errorf("duplicate across pages: %s", id)
		}
		seen[id] = true
	}
}

func TestSearchValidation(t *testing.T) {
	e := newTestEnv(t)
	viewer := mkProfile(t, e, map[string]any{})
	if code, _ := doSearch(t, e, viewer.TelegramUserID, "q="); code != http.StatusBadRequest {
		t.Errorf("empty q: %d", code)
	}
	if code, _ := doSearch(t, e, viewer.TelegramUserID, "q=x&page=0"); code != http.StatusBadRequest {
		t.Errorf("page=0: %d", code)
	}
	if code, _ := doSearch(t, e, viewer.TelegramUserID, "q=x&meeting_format=bad"); code != http.StatusBadRequest {
		t.Errorf("bad meeting_format: %d", code)
	}
}
