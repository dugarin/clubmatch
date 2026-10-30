package profile

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Kafka":              "kafka",
		"  KAFKA  ":          "kafka",
		"Кафка":              "кафка",
		"Ёлка":               "елка",
		"data   engineer":    "data engineer",
		"multi\n\tline text": "multi line text",
		"":                   "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func validInput() Input {
	return Input{
		FirstName:     "Иван",
		Skills:        []string{"Go"},
		MeetingFormat: "online",
	}
}

func TestValidateOK(t *testing.T) {
	if fields := Validate(validInput()); len(fields) != 0 {
		t.Errorf("valid input rejected: %v", fields)
	}
	full := validInput()
	full.LastName = "Петров"
	full.Projects = []string{"Проект номер один"}
	full.ProfessionalInterests = []string{"базы данных"}
	full.PersonalInterests = []string{"шахматы"}
	full.Location = "Москва"
	full.MeetingFormat = "any"
	if fields := Validate(full); len(fields) != 0 {
		t.Errorf("full valid input rejected: %v", fields)
	}
}

func TestValidateFailures(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Input)
		field  string
	}{
		{"first name empty", func(in *Input) { in.FirstName = "" }, "first_name"},
		{"first name 1 char", func(in *Input) { in.FirstName = "Я" }, "first_name"},
		{"first name 65 chars", func(in *Input) { in.FirstName = strings.Repeat("a", 65) }, "first_name"},
		{"first name spaces only", func(in *Input) { in.FirstName = "   " }, "first_name"},
		{"last name 65 chars", func(in *Input) { in.LastName = strings.Repeat("б", 65) }, "last_name"},
		{"no skills", func(in *Input) { in.Skills = nil }, "skills"},
		{"31 skills", func(in *Input) {
			in.Skills = make([]string, 31)
			for i := range in.Skills {
				in.Skills[i] = strings.Repeat("s", 2) + string(rune('a'+i%26)) + strings.Repeat("x", i/26)
			}
		}, "skills"},
		{"skill 1 char", func(in *Input) { in.Skills = []string{"g"} }, "skills"},
		{"skill 65 chars", func(in *Input) { in.Skills = []string{strings.Repeat("k", 65)} }, "skills"},
		{"duplicate skills exact", func(in *Input) { in.Skills = []string{"Go", "Go"} }, "skills"},
		{"duplicate skills normalized", func(in *Input) { in.Skills = []string{"Kafka", " kafka "} }, "skills"},
		{"duplicate skills ё", func(in *Input) { in.Skills = []string{"вёрстка", "верстка"} }, "skills"},
		{"21 projects", func(in *Input) {
			in.Projects = make([]string, 21)
			for i := range in.Projects {
				in.Projects[i] = strings.Repeat("p", 2) + string(rune('a'+i))
			}
		}, "projects"},
		{"project 1 char", func(in *Input) { in.Projects = []string{"x"} }, "projects"},
		{"project 257 chars", func(in *Input) { in.Projects = []string{strings.Repeat("p", 257)} }, "projects"},
		{"31 professional interests", func(in *Input) {
			in.ProfessionalInterests = make([]string, 31)
			for i := range in.ProfessionalInterests {
				in.ProfessionalInterests[i] = strings.Repeat("i", 2) + string(rune('a'+i%26)) + strings.Repeat("y", i/26)
			}
		}, "professional_interests"},
		{"duplicate personal interests", func(in *Input) { in.PersonalInterests = []string{"Шахматы", "шахматы"} }, "personal_interests"},
		{"location 1 char", func(in *Input) { in.Location = "М" }, "location"},
		{"location 65 chars", func(in *Input) { in.Location = strings.Repeat("м", 65) }, "location"},
		{"meeting format empty", func(in *Input) { in.MeetingFormat = "" }, "meeting_format"},
		{"meeting format unknown", func(in *Input) { in.MeetingFormat = "hybrid" }, "meeting_format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)
			fields := Validate(in)
			found := false
			for _, f := range fields {
				if f == tc.field {
					found = true
				}
			}
			if !found {
				t.Errorf("expected field %q in %v", tc.field, fields)
			}
		})
	}
}

func TestValidateBoundaries(t *testing.T) {
	in := validInput()
	in.FirstName = strings.Repeat("ж", 2)
	in.Skills = []string{strings.Repeat("ж", 64)}
	in.Projects = []string{strings.Repeat("ж", 256)}
	in.Location = strings.Repeat("ж", 64)
	if fields := Validate(in); len(fields) != 0 {
		t.Errorf("boundary values rejected: %v", fields)
	}
	in.Skills = make([]string, 30)
	for i := range in.Skills {
		in.Skills[i] = strings.Repeat("s", 2) + string(rune('a'+i%26)) + strings.Repeat("z", i/26)
	}
	if fields := Validate(in); len(fields) != 0 {
		t.Errorf("30 skills rejected: %v", fields)
	}
}
