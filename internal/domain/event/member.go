package event

// MemberContact is the enriched contact of a thread member as it travels in an
// event, so consumers do not have to call im-contact-service themselves.
type MemberContact struct {
	ID       string `json:"id"`
	Sub      string `json:"sub"`
	Iss      string `json:"iss,omitempty"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Type     string `json:"type"`
	IsBot    bool   `json:"is_bot"`
}

// Member identifies a thread participant by its membership id and carries the
// contact behind it. Role is the ThreadRole name, not its ordinal.
type Member struct {
	ID      string         `json:"id"`
	Contact *MemberContact `json:"contact,omitempty"`
	Role    string         `json:"role"`
}
