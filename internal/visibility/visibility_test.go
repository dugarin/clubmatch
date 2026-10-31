package visibility

import (
	"encoding/json"
	"testing"
)

func sampleCard() Card {
	avatar := "https://t.me/avatar.jpg"
	username := "ivan"
	var tgID int64 = 123456
	return Card{
		ID:                    "11111111-2222-3333-4444-555555555555",
		FirstName:             "Иван",
		LastName:              "Петров",
		Skills:                []string{"Go"},
		Projects:              []string{"Миграция"},
		ProfessionalInterests: []string{"Базы данных"},
		PersonalInterests:     []string{"Шахматы"},
		Location:              "Москва",
		MeetingFormat:         "online",
		AvatarURL:             &avatar,
		TelegramUsername:      &username,
		TelegramUserID:        &tgID,
	}
}

var hiddenPreMatch = []string{"first_name", "last_name", "avatar_url", "telegram_username", "telegram_user_id"}

func keysOf(t *testing.T, m map[string]any) map[string]bool {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	keys := make(map[string]bool, len(decoded))
	for k := range decoded {
		keys[k] = true
	}
	return keys
}

func TestSerializePreMatchHidesContactFields(t *testing.T) {
	for _, state := range []State{StatePreMatch, StateModerator} {
		t.Run(string(state), func(t *testing.T) {
			keys := keysOf(t, Serialize(sampleCard(), state))
			for _, hidden := range hiddenPreMatch {
				if keys[hidden] {
					t.Errorf("state %s: field %q must be absent", state, hidden)
				}
			}
			for _, want := range []string{"id", "skills", "projects", "professional_interests", "personal_interests", "location", "meeting_format"} {
				if !keys[want] {
					t.Errorf("state %s: field %q must be present", state, want)
				}
			}
		})
	}
}

func TestSerializeMatchedDisclosesContact(t *testing.T) {
	m := Serialize(sampleCard(), StateMatched)
	if m["first_name"] != "Иван" || m["last_name"] != "Петров" {
		t.Errorf("name = %v %v", m["first_name"], m["last_name"])
	}
	if m["telegram_user_id"] != int64(123456) {
		t.Errorf("telegram_user_id = %v", m["telegram_user_id"])
	}
	if m["telegram_username"] != "ivan" {
		t.Errorf("telegram_username = %v", m["telegram_username"])
	}
	if m["avatar_url"] != "https://t.me/avatar.jpg" {
		t.Errorf("avatar_url = %v", m["avatar_url"])
	}
	if m["skills"].([]string)[0] != "Go" {
		t.Errorf("skills = %v", m["skills"])
	}
}

func TestSerializeMatchedNullables(t *testing.T) {
	c := sampleCard()
	c.AvatarURL = nil
	c.TelegramUsername = nil
	m := Serialize(c, StateMatched)
	v, present := m["telegram_username"]
	if !present || v != nil {
		t.Errorf("telegram_username = %v (present=%v), want explicit null", v, present)
	}
	if _, present := m["avatar_url"]; !present {
		t.Error("avatar_url must be present (null placeholder)")
	}
}

func TestSerializeSelf(t *testing.T) {
	keys := keysOf(t, Serialize(sampleCard(), StateSelf))
	for _, want := range []string{"id", "skills", "first_name", "telegram_user_id"} {
		if !keys[want] {
			t.Errorf("self: field %q must be present", want)
		}
	}
}

func TestSerializeMatchedDeleted(t *testing.T) {
	if out := Serialize(sampleCard(), StateMatchedDeleted); out != nil {
		t.Errorf("matched_deleted must yield nil card, got %v", out)
	}
}
