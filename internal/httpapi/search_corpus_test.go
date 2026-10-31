package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/clubmatch/clubmatch/internal/profile"
	"github.com/clubmatch/clubmatch/internal/store"
)

// Корпус AC-5 — docs/spikes/ac5-corpus.md (T-3, задача 3.1.2).

func TestSearchCorpusAC5(t *testing.T) {
	e := newTestEnv(t)
	sfx := "Corpus" + runID

	p1 := mkProfile(t, e, map[string]any{"skills": []string{"Kafka " + sfx}})
	// Кросс-скриптовый алиас «Кафка» — данность справочника (AC-5, D-3.1): добавляем напрямую.
	tagRow, err := e.st.GetTagByNormalized(context.Background(), profile.Normalize("Kafka "+sfx))
	if err != nil {
		t.Fatalf("find tag: %v", err)
	}
	alias := "Кафка " + sfx
	lex, err := e.st.LexemesOfText(context.Background(), alias)
	if err != nil {
		t.Fatalf("lexemes: %v", err)
	}
	if err := e.st.CreateTagAlias(context.Background(), store.CreateTagAliasParams{
		TagID:      tagRow.ID,
		Alias:      alias,
		Normalized: profile.Normalize(alias),
		Lexemes:    lex,
	}); err != nil {
		t.Fatalf("create alias: %v", err)
	}
	p2 := mkProfile(t, e, map[string]any{"skills": []string{"Skill" + sfx}, "projects": []string{"Миграция на Kafka " + sfx}})
	p3 := mkProfile(t, e, map[string]any{"skills": []string{"Skill" + sfx + "3"}, "professional_interests": []string{"Kafka " + sfx}})
	p4 := mkProfile(t, e, map[string]any{"skills": []string{"Skill" + sfx + "4"}, "personal_interests": []string{"Kafka " + sfx}})
	p5 := mkProfile(t, e, map[string]any{"skills": []string{"Go " + sfx}})
	viewer := mkProfile(t, e, map[string]any{"skills": []string{"Viewer" + sfx}})

	search := func(q string) []string {
		t.Helper()
		code, res := doSearch(t, e, viewer.TelegramUserID, "q="+url.QueryEscape(q))
		if code != http.StatusOK {
			t.Fatalf("search %q: %d", q, code)
		}
		ids := make([]string, 0, len(res.Items))
		for _, it := range res.Items {
			ids = append(ids, it["id"].(string))
		}
		return ids
	}

	ids := search("Kafka " + sfx)
	// spec 1.2 (tag-only): анкеты с тегом — по приоритету полей skill > prof > personal;
	// анкета с упоминанием в тексте проекта (p2) НЕ находится (полнотекст — post-MVP).
	if len(ids) != 3 {
		t.Fatalf("C1: expected 3 tag profiles, got %v", ids)
	}
	if ids[0] != uuidString(p1.ID) || indexOf(ids, uuidString(p3.ID)) < 0 || indexOf(ids, uuidString(p4.ID)) < 0 {
		t.Fatalf("C1: %v", ids)
	}
	if indexOf(ids, uuidString(p2.ID)) >= 0 {
		t.Errorf("C1: text-only profile must not match (spec 1.2): %v", ids)
	}
	if indexOf(ids, uuidString(p3.ID)) > indexOf(ids, uuidString(p4.ID)) {
		t.Errorf("C7: professional_interest must outrank personal_interest: %v", ids)
	}

	ids = search("разобраться с кафкой " + sfx)
	if len(ids) == 0 || ids[0] != uuidString(p1.ID) {
		t.Fatalf("C2 (flagship AC-5): tag profile must be first for «разобраться с кафкой»: %v", ids)
	}
	if indexOf(ids, uuidString(p5.ID)) >= 0 {
		t.Errorf("C2: unrelated profile must not appear: %v", ids)
	}

	ids = search("кафкой " + sfx)
	if len(ids) == 0 || ids[0] != uuidString(p1.ID) {
		t.Fatalf("C3: stemmed term must hit tag alias: %v", ids)
	}

	ids = search("Go " + sfx)
	if len(ids) != 1 || ids[0] != uuidString(p5.ID) {
		t.Fatalf("C4: only Go profile expected: %v", ids)
	}

	code, res := doSearch(t, e, viewer.TelegramUserID, "q=no-such-term-"+runID)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("C5: unknown tag must yield 422 (spec 1.2), got %d", code)
	}

	code, res = doSearch(t, e, viewer.TelegramUserID, "q="+url.QueryEscape("Kafka "+sfx)+"&location=nowhere-"+runID)
	if code != http.StatusOK || len(res.Items) != 0 || !res.FiltersApplied {
		t.Fatalf("C6: filtered-out result: code=%d items=%d filters=%v", code, len(res.Items), res.FiltersApplied)
	}
}
