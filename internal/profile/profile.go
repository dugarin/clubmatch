package profile

import (
	"strings"
	"unicode/utf8"
)

type Input struct {
	FirstName             string
	LastName              string
	Skills                []string
	Projects              []string
	ProfessionalInterests []string
	PersonalInterests     []string
	Location              string
	MeetingFormat         string
}

var meetingFormats = map[string]bool{"online": true, "offline": true, "any": true}

func Normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return strings.Join(strings.Fields(s), " ")
}

func Validate(in Input) []string {
	var fields []string
	if !validString(in.FirstName, 2, 64) {
		fields = append(fields, "first_name")
	}
	if !validString(in.LastName, 0, 64) {
		fields = append(fields, "last_name")
	}
	if !validList(in.Skills, 1, 30, 2, 64) {
		fields = append(fields, "skills")
	}
	if !validList(in.Projects, 0, 20, 2, 256) {
		fields = append(fields, "projects")
	}
	if !validList(in.ProfessionalInterests, 0, 30, 2, 64) {
		fields = append(fields, "professional_interests")
	}
	if !validList(in.PersonalInterests, 0, 30, 2, 64) {
		fields = append(fields, "personal_interests")
	}
	if in.Location != "" && !validString(in.Location, 2, 64) {
		fields = append(fields, "location")
	}
	if !meetingFormats[in.MeetingFormat] {
		fields = append(fields, "meeting_format")
	}
	return fields
}

func validString(s string, minLen, maxLen int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	return n >= minLen && n <= maxLen
}

func validList(items []string, minItems, maxItems, minLen, maxLen int) bool {
	if len(items) < minItems || len(items) > maxItems {
		return false
	}
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		if !validString(it, minLen, maxLen) {
			return false
		}
		n := Normalize(it)
		if seen[n] {
			return false
		}
		seen[n] = true
	}
	return true
}
