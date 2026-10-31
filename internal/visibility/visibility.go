// Package visibility implements the field-visibility policy (FR-14, ADR-004):
// a single serializer that emits exactly the fields allowed for the
// viewer↔owner relation. Hidden fields are absent from the output (AC-7).
package visibility

type State string

const (
	StateSelf           State = "self"
	StatePreMatch       State = "pre_match"
	StateMatched        State = "matched"
	StateMatchedDeleted State = "matched_deleted"
	StateModerator      State = "moderator"
)

type Card struct {
	ID                    string
	FirstName             string
	LastName              string
	Skills                []string
	Projects              []string
	ProfessionalInterests []string
	PersonalInterests     []string
	Location              string
	MeetingFormat         string
	AvatarURL             *string
	TelegramUsername      *string
	TelegramUserID        *int64
}

func strOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Serialize returns the card as a JSON-ready map containing only the fields
// visible in the given state. StateMatchedDeleted yields nil: the card must
// not be disclosed at all (FR-17, ADR-004 п.1).
func Serialize(c Card, s State) map[string]any {
	if s == StateMatchedDeleted {
		return nil
	}
	out := map[string]any{
		"id":                     c.ID,
		"skills":                 strOrEmpty(c.Skills),
		"projects":               strOrEmpty(c.Projects),
		"professional_interests": strOrEmpty(c.ProfessionalInterests),
		"personal_interests":     strOrEmpty(c.PersonalInterests),
		"meeting_format":         c.MeetingFormat,
	}
	if c.Location != "" {
		out["location"] = c.Location
	}
	if s == StatePreMatch || s == StateModerator {
		return out
	}
	out["first_name"] = c.FirstName
	if c.LastName != "" {
		out["last_name"] = c.LastName
	}
	if c.AvatarURL != nil {
		out["avatar_url"] = *c.AvatarURL
	} else {
		out["avatar_url"] = nil
	}
	if c.TelegramUsername != nil {
		out["telegram_username"] = *c.TelegramUsername
	} else {
		out["telegram_username"] = nil
	}
	if c.TelegramUserID != nil {
		out["telegram_user_id"] = *c.TelegramUserID
	}
	return out
}
