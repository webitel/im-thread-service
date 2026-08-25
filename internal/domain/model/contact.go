package model

// ContactIdentity is the im-contact-service view of a contact, resolved when an
// event has to carry more than the contact id.
type ContactIdentity struct {
	Sub      string
	Issuer   string
	Type     string
	Name     string
	Username string
	IsBot    bool
}
