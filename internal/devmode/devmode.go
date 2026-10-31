package devmode

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/clubmatch/clubmatch/internal/auth"
	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DemoTelegramUserID int64 = 900000000001
	DemoUsername             = "demo_alex"
)

type demoTag struct {
	canonical  string
	normalized string
	kind       store.ProfileTagKind
}

type demoUser struct {
	telegramID int64
	username   string
	firstName  string
	lastName   string
	location   string
	format     store.MeetingFormat
	tags       []demoTag
	projects   []string
}

var demoUsers = []demoUser{
	{
		telegramID: DemoTelegramUserID,
		username:   DemoUsername,
		firstName:  "Alex",
		lastName:   "Demo",
		location:   "Moscow",
		format:     store.MeetingFormatAny,
		tags: []demoTag{
			{canonical: "Go", normalized: "go", kind: store.ProfileTagKindSkill},
			{canonical: "Kafka", normalized: "kafka", kind: store.ProfileTagKindSkill},
			{canonical: "PostgreSQL", normalized: "postgresql", kind: store.ProfileTagKindProfessionalInterest},
			{canonical: "Cycling", normalized: "cycling", kind: store.ProfileTagKindPersonalInterest},
		},
		projects: []string{"Community matching platform"},
	},
	{
		telegramID: 900000000002,
		username:   "demo_maria",
		firstName:  "Maria",
		lastName:   "Kafka",
		location:   "Moscow",
		format:     store.MeetingFormatAny,
		tags: []demoTag{
			{canonical: "Kafka", normalized: "kafka", kind: store.ProfileTagKindSkill},
			{canonical: "Go", normalized: "go", kind: store.ProfileTagKindSkill},
			{canonical: "PostgreSQL", normalized: "postgresql", kind: store.ProfileTagKindProfessionalInterest},
			{canonical: "Reading", normalized: "reading", kind: store.ProfileTagKindPersonalInterest},
		},
		projects: []string{"Reliable event pipeline with Kafka"},
	},
	{
		telegramID: 900000000003,
		username:   "demo_ivan",
		firstName:  "Ivan",
		lastName:   "Frontend",
		location:   "Remote",
		format:     store.MeetingFormatOnline,
		tags: []demoTag{
			{canonical: "TypeScript", normalized: "typescript", kind: store.ProfileTagKindSkill},
			{canonical: "React", normalized: "react", kind: store.ProfileTagKindSkill},
			{canonical: "Mini Apps", normalized: "mini apps", kind: store.ProfileTagKindProfessionalInterest},
			{canonical: "Hiking", normalized: "hiking", kind: store.ProfileTagKindPersonalInterest},
		},
		projects: []string{"Telegram Mini App interface"},
	},
	{
		telegramID: 900000000004,
		username:   "demo_olga",
		firstName:  "Olga",
		lastName:   "Database",
		location:   "Berlin",
		format:     store.MeetingFormatOffline,
		tags: []demoTag{
			{canonical: "PostgreSQL", normalized: "postgresql", kind: store.ProfileTagKindSkill},
			{canonical: "Go", normalized: "go", kind: store.ProfileTagKindSkill},
			{canonical: "SRE", normalized: "sre", kind: store.ProfileTagKindProfessionalInterest},
			{canonical: "Cycling", normalized: "cycling", kind: store.ProfileTagKindPersonalInterest},
		},
		projects: []string{"PostgreSQL observability and backup automation"},
	},
}

func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := auth.Identity{
				TelegramUserID: DemoTelegramUserID,
				Username:       DemoUsername,
				FirstName:      "Alex",
				LastName:       "Demo",
				AuthDate:       time.Now(),
			}
			next.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), id)))
		})
	}
}

func Seed(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin dev seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	st := store.NewStore(tx)
	ids := make(map[int64]pgtype.UUID, len(demoUsers))

	for _, d := range demoUsers {
		username := d.username
		u, err := st.EnsureDevUser(ctx, store.EnsureDevUserParams{
			TelegramUserID:   d.telegramID,
			TelegramUsername: &username,
		})
		if err != nil {
			return fmt.Errorf("seed user %s: %w", d.username, err)
		}
		ids[d.telegramID] = u.ID
		if err := st.EnsureDevConsent(ctx, u.ID); err != nil {
			return fmt.Errorf("seed consent %s: %w", d.username, err)
		}
		lastName := d.lastName
		location := d.location
		locationNorm := strings.ToLower(d.location)
		if err := st.UpsertProfile(ctx, store.UpsertProfileParams{
			UserID:        u.ID,
			FirstName:     d.firstName,
			LastName:      &lastName,
			Location:      &location,
			LocationNorm:  &locationNorm,
			MeetingFormat: d.format,
		}); err != nil {
			return fmt.Errorf("seed profile %s: %w", d.username, err)
		}
		for _, dt := range d.tags {
			tag, err := st.EnsureDevTag(ctx, store.EnsureDevTagParams{
				Canonical:  dt.canonical,
				Normalized: dt.normalized,
				CreatedBy:  u.ID,
			})
			if err != nil {
				return fmt.Errorf("seed tag %s: %w", dt.canonical, err)
			}
			lexemes, err := st.LexemesOfText(ctx, dt.normalized)
			if err != nil {
				return fmt.Errorf("seed tag lexemes %s: %w", dt.canonical, err)
			}
			if err := st.CreateTagAlias(ctx, store.CreateTagAliasParams{
				TagID:      tag.ID,
				Alias:      dt.canonical,
				Normalized: dt.normalized,
				Lexemes:    lexemes,
			}); err != nil {
				return fmt.Errorf("seed tag alias %s: %w", dt.canonical, err)
			}
			if err := st.InsertProfileTag(ctx, store.InsertProfileTagParams{
				ProfileID: u.ID,
				TagID:     tag.ID,
				Kind:      dt.kind,
			}); err != nil {
				return fmt.Errorf("seed profile tag %s: %w", dt.canonical, err)
			}
		}
		for i, project := range d.projects {
			if err := st.EnsureDevProject(ctx, store.EnsureDevProjectParams{
				ProfileID: u.ID,
				Text:      project,
				Position:  int16(i),
			}); err != nil {
				return fmt.Errorf("seed project %s: %w", d.username, err)
			}
		}
		if err := st.RefreshProfileSearchable(ctx, u.ID); err != nil {
			return fmt.Errorf("seed searchable %s: %w", d.username, err)
		}
	}

	viewer := ids[DemoTelegramUserID]
	matched := ids[900000000002]
	low, high := viewer, matched
	if bytes.Compare(low.Bytes[:], high.Bytes[:]) > 0 {
		low, high = high, low
	}
	for _, evaluation := range []store.UpsertEvaluationParams{
		{EvaluatorID: viewer, TargetID: matched, Value: store.EvaluationValueInterested, Source: store.EvaluationSourceDeck},
		{EvaluatorID: matched, TargetID: viewer, Value: store.EvaluationValueInterested, Source: store.EvaluationSourceDeck},
	} {
		if err := st.UpsertEvaluation(ctx, evaluation); err != nil {
			return fmt.Errorf("seed evaluation: %w", err)
		}
	}
	if err := st.EnsureDevMatch(ctx, store.EnsureDevMatchParams{UserLow: low, UserHigh: high}); err != nil {
		return fmt.Errorf("seed match: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit dev seed: %w", err)
	}
	return nil
}
