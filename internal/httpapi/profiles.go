package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/clubmatch/clubmatch/internal/profile"
	"github.com/clubmatch/clubmatch/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProfileHandler struct {
	pool *pgxpool.Pool
	st   *store.Store
}

func NewProfileHandler(pool *pgxpool.Pool, st *store.Store) *ProfileHandler {
	return &ProfileHandler{pool: pool, st: st}
}

type profileInputDTO struct {
	FirstName             *string   `json:"first_name"`
	LastName              *string   `json:"last_name"`
	Skills                *[]string `json:"skills"`
	Projects              *[]string `json:"projects"`
	ProfessionalInterests *[]string `json:"professional_interests"`
	PersonalInterests     *[]string `json:"personal_interests"`
	Location              *string   `json:"location"`
	MeetingFormat         *string   `json:"meeting_format"`
}

type createProfileDTO struct {
	profileInputDTO
	ConsentVersion string `json:"consent_version"`
	InviteToken    string `json:"invite_token"`
}

type profileSelfDTO struct {
	FirstName             string   `json:"first_name"`
	LastName              string   `json:"last_name,omitempty"`
	Skills                []string `json:"skills"`
	Projects              []string `json:"projects"`
	ProfessionalInterests []string `json:"professional_interests"`
	PersonalInterests     []string `json:"personal_interests"`
	Location              string   `json:"location,omitempty"`
	MeetingFormat         string   `json:"meeting_format"`
	IsSearchable          bool     `json:"is_searchable"`
	BotLinked             bool     `json:"bot_linked"`
	UpdatedAt             string   `json:"updated_at"`
}

type tagDTO struct {
	ID        string `json:"id"`
	Canonical string `json:"canonical"`
}

type errorDTO struct {
	Error  string   `json:"error"`
	Fields []string `json:"fields,omitempty"`
}

func writeErr(w http.ResponseWriter, code int, msg string, fields ...string) {
	writeJSON(w, code, errorDTO{Error: msg, Fields: fields})
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strSliceVal(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}

func (dto profileInputDTO) toInput() profile.Input {
	return profile.Input{
		FirstName:             strVal(dto.FirstName),
		LastName:              strVal(dto.LastName),
		Skills:                strSliceVal(dto.Skills),
		Projects:              strSliceVal(dto.Projects),
		ProfessionalInterests: strSliceVal(dto.ProfessionalInterests),
		PersonalInterests:     strSliceVal(dto.PersonalInterests),
		Location:              strVal(dto.Location),
		MeetingFormat:         strVal(dto.MeetingFormat),
	}
}

// validatePatch checks only the fields present in the PATCH body; fields absent
// keep their stored values, which already satisfy the invariants.
func validatePatch(dto profileInputDTO) []string {
	var fields []string
	probe := profile.Input{FirstName: "ok", Skills: []string{"ok"}, MeetingFormat: "any"}
	if dto.FirstName != nil {
		probe.FirstName = *dto.FirstName
	}
	if dto.LastName != nil {
		probe.LastName = *dto.LastName
	}
	if dto.Skills != nil {
		probe.Skills = *dto.Skills
	}
	if dto.Projects != nil {
		probe.Projects = *dto.Projects
	}
	if dto.ProfessionalInterests != nil {
		probe.ProfessionalInterests = *dto.ProfessionalInterests
	}
	if dto.PersonalInterests != nil {
		probe.PersonalInterests = *dto.PersonalInterests
	}
	if dto.Location != nil {
		probe.Location = *dto.Location
	}
	if dto.MeetingFormat != nil {
		probe.MeetingFormat = *dto.MeetingFormat
	}
	for _, f := range profile.Validate(probe) {
		switch f {
		case "first_name":
			if dto.FirstName != nil {
				fields = append(fields, f)
			}
		case "last_name":
			fields = append(fields, f)
		case "skills":
			if dto.Skills != nil {
				fields = append(fields, f)
			}
		case "projects":
			if dto.Projects != nil {
				fields = append(fields, f)
			}
		case "professional_interests":
			if dto.ProfessionalInterests != nil {
				fields = append(fields, f)
			}
		case "personal_interests":
			if dto.PersonalInterests != nil {
				fields = append(fields, f)
			}
		case "location":
			fields = append(fields, f)
		case "meeting_format":
			if dto.MeetingFormat != nil {
				fields = append(fields, f)
			}
		}
	}
	return fields
}

func uuidString(u pgtype.UUID) string {
	b := u.Bytes
	var buf [36]byte
	const hexDigits = "0123456789abcdef"
	j := 0
	for i := 0; i < 16; i++ {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			buf[j] = '-'
			j++
		}
		buf[j] = hexDigits[b[i]>>4]
		buf[j+1] = hexDigits[b[i]&0x0f]
		j += 2
	}
	return string(buf[:])
}

func (h *ProfileHandler) selfDTO(ctx context.Context, userID pgtype.UUID) (profileSelfDTO, error) {
	row, err := h.st.GetProfileSelf(ctx, userID)
	if err != nil {
		return profileSelfDTO{}, err
	}
	out := profileSelfDTO{
		FirstName:             row.FirstName,
		LastName:              strVal(row.LastName),
		Skills:                []string{},
		Projects:              []string{},
		ProfessionalInterests: []string{},
		PersonalInterests:     []string{},
		Location:              strVal(row.Location),
		MeetingFormat:         string(row.MeetingFormat),
		IsSearchable:          row.IsSearchable,
		BotLinked:             row.BotLinked,
		UpdatedAt:             row.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
	kinds := []struct {
		kind store.ProfileTagKind
		dst  *[]string
	}{
		{store.ProfileTagKindSkill, &out.Skills},
		{store.ProfileTagKindProfessionalInterest, &out.ProfessionalInterests},
		{store.ProfileTagKindPersonalInterest, &out.PersonalInterests},
	}
	for _, k := range kinds {
		list, err := h.st.ListProfileTagCanonicals(ctx, store.ListProfileTagCanonicalsParams{
			ProfileID: userID,
			Kind:      k.kind,
		})
		if err != nil {
			return profileSelfDTO{}, err
		}
		if list != nil {
			*k.dst = list
		}
	}
	projects, err := h.st.ListProjectTexts(ctx, userID)
	if err != nil {
		return profileSelfDTO{}, err
	}
	if projects != nil {
		out.Projects = projects
	}
	return out, nil
}

func upsertProfileFields(ctx context.Context, qtx *store.Queries, userID pgtype.UUID, in profile.Input) error {
	var lastName, location, locationNorm *string
	if v := strings.TrimSpace(in.LastName); v != "" {
		lastName = &v
	}
	if v := strings.TrimSpace(in.Location); v != "" {
		location = &v
		n := profile.Normalize(in.Location)
		locationNorm = &n
	}
	return qtx.UpsertProfile(ctx, store.UpsertProfileParams{
		UserID:        userID,
		FirstName:     strings.TrimSpace(in.FirstName),
		LastName:      lastName,
		Location:      location,
		LocationNorm:  locationNorm,
		MeetingFormat: store.MeetingFormat(in.MeetingFormat),
		IsSearchable:  true,
	})
}

func replaceTags(ctx context.Context, qtx *store.Queries, userID pgtype.UUID, kind store.ProfileTagKind, values []string) error {
	if err := qtx.DeleteProfileTagsByKind(ctx, store.DeleteProfileTagsByKindParams{
		ProfileID: userID,
		Kind:      kind,
	}); err != nil {
		return err
	}
	seen := make(map[pgtype.UUID]bool, len(values))
	for _, v := range values {
		t, err := store.GetOrCreateTag(ctx, qtx, userID, v, profile.Normalize(v))
		if err != nil {
			return err
		}
		if seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		if err := qtx.InsertProfileTag(ctx, store.InsertProfileTagParams{
			ProfileID: userID,
			TagID:     t.ID,
			Kind:      kind,
		}); err != nil {
			return err
		}
	}
	return nil
}

func replaceProjects(ctx context.Context, qtx *store.Queries, userID pgtype.UUID, projects []string) error {
	if err := qtx.DeleteProjectsByProfileID(ctx, userID); err != nil {
		return err
	}
	for i, p := range projects {
		if err := qtx.InsertProject(ctx, store.InsertProjectParams{
			ProfileID: userID,
			Text:      strings.TrimSpace(p),
			Position:  int16(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h *ProfileHandler) currentUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	return currentUser(h.st, w, r)
}

func (h *ProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	var dto createProfileDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	useInvite := u.AccessState != store.UserAccessStateApproved && u.AccessState != store.UserAccessStateInvited
	if useInvite && dto.InviteToken == "" {
		writeErr(w, http.StatusForbidden, "no access: invite or approval required")
		return
	}
	if strings.TrimSpace(dto.ConsentVersion) == "" {
		writeErr(w, http.StatusForbidden, "consent required")
		return
	}
	in := dto.toInput()
	if fields := profile.Validate(in); len(fields) > 0 {
		writeErr(w, http.StatusUnprocessableEntity, "validation failed", fields...)
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("profiles: begin tx", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.st.WithTx(tx)

	if useInvite {
		rows, err := qtx.MarkInviteUsed(ctx, store.MarkInviteUsedParams{
			Token:  dto.InviteToken,
			UsedBy: u.ID,
		})
		if err != nil {
			slog.Error("profiles: mark invite used", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		if rows == 0 {
			writeErr(w, http.StatusForbidden, "invite token invalid or already used")
			return
		}
	}
	if err := upsertProfileFields(ctx, qtx, u.ID, in); err != nil {
		slog.Error("profiles: upsert profile", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, spec := range []struct {
		kind   store.ProfileTagKind
		values []string
	}{
		{store.ProfileTagKindSkill, in.Skills},
		{store.ProfileTagKindProfessionalInterest, in.ProfessionalInterests},
		{store.ProfileTagKindPersonalInterest, in.PersonalInterests},
	} {
		if err := replaceTags(ctx, qtx, u.ID, spec.kind, spec.values); err != nil {
			slog.Error("profiles: replace tags", "kind", spec.kind, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if err := replaceProjects(ctx, qtx, u.ID, in.Projects); err != nil {
		slog.Error("profiles: replace projects", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.InsertConsent(ctx, store.InsertConsentParams{
		UserID:          u.ID,
		DocumentVersion: strings.TrimSpace(dto.ConsentVersion),
	}); err != nil {
		slog.Error("profiles: insert consent", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.RebuildProfileFTS(ctx, u.ID); err != nil {
		slog.Error("profiles: rebuild fts", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.SetUserOnboarded(ctx, u.ID); err != nil {
		slog.Error("profiles: set onboarded", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("profiles: commit", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := h.selfDTO(ctx, u.ID)
	if err != nil {
		slog.Error("profiles: load self", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *ProfileHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	u, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	out, err := h.selfDTO(r.Context(), u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		slog.Error("profiles: load self", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ProfileHandler) PatchMe(w http.ResponseWriter, r *http.Request) {
	u, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	var dto profileInputDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx := r.Context()
	cur, err := h.st.GetProfileByUserID(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		slog.Error("profiles: load profile", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if fields := validatePatch(dto); len(fields) > 0 {
		writeErr(w, http.StatusUnprocessableEntity, "validation failed", fields...)
		return
	}

	in := profile.Input{
		FirstName:     cur.FirstName,
		LastName:      strVal(cur.LastName),
		Location:      strVal(cur.Location),
		MeetingFormat: string(cur.MeetingFormat),
	}
	if dto.FirstName != nil {
		in.FirstName = *dto.FirstName
	}
	if dto.LastName != nil {
		in.LastName = *dto.LastName
	}
	if dto.Location != nil {
		in.Location = *dto.Location
	}
	if dto.MeetingFormat != nil {
		in.MeetingFormat = *dto.MeetingFormat
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("profiles: begin tx", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.st.WithTx(tx)

	if err := upsertProfileFields(ctx, qtx, u.ID, in); err != nil {
		slog.Error("profiles: upsert profile", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, spec := range []struct {
		patch *[]string
		kind  store.ProfileTagKind
	}{
		{dto.Skills, store.ProfileTagKindSkill},
		{dto.ProfessionalInterests, store.ProfileTagKindProfessionalInterest},
		{dto.PersonalInterests, store.ProfileTagKindPersonalInterest},
	} {
		if spec.patch == nil {
			continue
		}
		if err := replaceTags(ctx, qtx, u.ID, spec.kind, *spec.patch); err != nil {
			slog.Error("profiles: replace tags", "kind", spec.kind, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if dto.Projects != nil {
		if err := replaceProjects(ctx, qtx, u.ID, *dto.Projects); err != nil {
			slog.Error("profiles: replace projects", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if err := qtx.RebuildProfileFTS(ctx, u.ID); err != nil {
		slog.Error("profiles: rebuild fts", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("profiles: commit", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := h.selfDTO(ctx, u.ID)
	if err != nil {
		slog.Error("profiles: load self", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ProfileHandler) Autocomplete(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.currentUser(w, r); !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if n := utf8.RuneCountInString(q); n < 1 || n > 64 {
		writeErr(w, http.StatusBadRequest, "q must be 1..64 characters")
		return
	}
	rows, err := h.st.AutocompleteTags(r.Context(), profile.Normalize(q))
	if err != nil {
		slog.Error("tags: autocomplete", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]tagDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, tagDTO{ID: uuidString(row.ID), Canonical: row.Canonical})
	}
	writeJSON(w, http.StatusOK, out)
}
