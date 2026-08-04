package service

// ReactionCapabilities decides whether a reaction emoji may be forwarded to a
// given gate's external messenger. The gateID parameter is the seam for a
// future per-provider capability source (resolved from im-providers-service);
// today the decision is a server-wide allow-list.
type ReactionCapabilities interface {
	Allowed(gateID, emoji string) bool
}

// staticReactionCapabilities enforces a server-wide allow-list: an empty list
// means unrestricted (each provider still silently drops what it cannot
// render). It is gate-agnostic for now.
type staticReactionCapabilities struct {
	allowed map[string]struct{}
}

// NewStaticReactionCapabilities builds a capability check from a fixed allow-list.
func NewStaticReactionCapabilities(allowed []string) ReactionCapabilities {
	set := make(map[string]struct{}, len(allowed))
	for _, e := range allowed {
		set[e] = struct{}{}
	}

	return &staticReactionCapabilities{allowed: set}
}

func (c *staticReactionCapabilities) Allowed(_ /* gateID */, emoji string) bool {
	if len(c.allowed) == 0 {
		return true
	}

	_, ok := c.allowed[emoji]

	return ok
}
