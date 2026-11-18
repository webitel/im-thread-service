package model

import (
	"strings"
	"unicode/utf8"
)

// PhoneDigits keeps only the digits of a phone number: CRM numbers and values
// typed by clients differ in "+", spaces, dashes and brackets.
func PhoneDigits(raw string) string {
	var b strings.Builder

	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// minSuffixDigits is the shortest number accepted as the tail of a longer one.
const minSuffixDigits = 7

// PhoneMatches reports whether a stored CRM number matches the wanted digits.
// With lastN > 0 the last N digits must be equal (a suffix match anchored to
// the end, unlike the CRM's substring search). Without it the shorter number
// must be the tail of the longer one and hold at least 7 digits, so
// +380000000067 matches 0000000067 (a trunk prefix instead of the country
// code) but not 1234567 inside another number's middle.
func PhoneMatches(stored, wantedDigits string, lastN int) bool {
	have := PhoneDigits(stored)
	if have == "" || wantedDigits == "" {
		return false
	}

	if lastN <= 0 {
		if have == wantedDigits {
			return true
		}

		short, long := have, wantedDigits
		if len(short) > len(long) {
			short, long = long, short
		}

		return len(short) >= minSuffixDigits && strings.HasSuffix(long, short)
	}

	if len(have) < lastN || len(wantedDigits) < lastN {
		return false
	}

	return have[len(have)-lastN:] == wantedDigits[len(wantedDigits)-lastN:]
}

// EmailMatches compares emails case-insensitively, ignoring surrounding space.
func EmailMatches(stored, wanted string) bool {
	stored, wanted = strings.TrimSpace(stored), strings.TrimSpace(wanted)

	return stored != "" && strings.EqualFold(stored, wanted)
}

// MaskPhone keeps the last two digits: "+380 00 000 00 67" -> "*********67".
func MaskPhone(raw string) string {
	digits := PhoneDigits(raw)
	if len(digits) <= 2 {
		return strings.Repeat("*", len(digits))
	}

	return strings.Repeat("*", len(digits)-2) + digits[len(digits)-2:]
}

// MaskEmail keeps the first character of the local part and the domain:
// "qa@example.com" -> "q***@example.com".
func MaskEmail(raw string) string {
	local, domain, ok := strings.Cut(strings.TrimSpace(raw), "@")
	if !ok || local == "" {
		return "***"
	}

	first := []rune(local)[0]

	// The domain may name the person or their company: keep its first letter
	// and the top-level part only.
	tld := ""
	if i := strings.LastIndex(domain, "."); i > 0 {
		tld = domain[i:]
	}

	hint := ""
	if domain != "" {
		hint = string([]rune(domain)[0])
	}

	return string(first) + "***@" + hint + "***" + tld
}

// MaskValue masks a criterion value for logs: phones and emails keep a
// short hint, anything else keeps only its length.
func MaskValue(t CriterionType, raw string) string {
	switch t {
	case CriterionPhone:
		return MaskPhone(raw)
	case CriterionEmail:
		return MaskEmail(raw)
	case CriterionUnspecified, CriterionAttribute, CriterionMessengerID:
		return strings.Repeat("*", min(len([]rune(raw)), 8))
	default:
		return strings.Repeat("*", min(len([]rune(raw)), 8))
	}
}

// MaxAttributeKeyLen bounds an attribute key, like the API does.
const MaxAttributeKeyLen = 64

// IsSearchableKey reports whether an attribute key can be sent to the CRM
// search as "key=value": non-empty, bounded, and free of the characters the
// CRM reads as search operators ("*", "%", "\\", "=", a leading "/"). Any other
// name a CRM variable may carry, spaces included, is accepted.
func IsSearchableKey(key string) bool {
	if key == "" || utf8.RuneCountInString(key) > MaxAttributeKeyLen || strings.HasPrefix(key, "/") {
		return false
	}

	return !strings.ContainsAny(key, "*%\\=")
}
