package notify

import "encoding/json"

// Схемы notification.payload по типам (D-3.11, ADR-002).
// Правила: ключи snake_case; идентификаторы — строки UUID; состав фиксирован,
// новые поля добавляются только опционально (воркер игнорирует неизвестные ключи).

type MatchCreatedPayload struct {
	MatchID string `json:"match_id"`
}

type ApplicationDecidedPayload struct {
	ApplicationID string `json:"application_id"`
}

type TagRevokedPayload struct {
	Tag string `json:"tag"`
}

// LastSkillLostPayload — тот же состав, что у TagRevokedPayload:
// tag — отозванный тег, из-за которого профиль потерял последний навык.
type LastSkillLostPayload = TagRevokedPayload

func MarshalPayload(v any) ([]byte, error) {
	return json.Marshal(v)
}
