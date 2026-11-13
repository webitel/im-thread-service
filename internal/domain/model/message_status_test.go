package model

import (
	"reflect"
	"testing"
)

func TestNormalizeDeliveryErrorCode(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		DeliveryErrorFileTooLarge:         DeliveryErrorFileTooLarge,
		DeliveryErrorFileUnavailable:      DeliveryErrorFileUnavailable,
		DeliveryErrorUnsupportedMedia:     DeliveryErrorUnsupportedMedia,
		DeliveryErrorRecipientUnavailable: DeliveryErrorRecipientUnavailable,
		DeliveryErrorChannelUnavailable:   DeliveryErrorChannelUnavailable,
		DeliveryErrorSendFailed:           DeliveryErrorSendFailed,
		"403":                             DeliveryErrorSendFailed,
		"131047":                          DeliveryErrorSendFailed,
		"FILE_TOO_LARGE":                  DeliveryErrorSendFailed,
		"":                                DeliveryErrorSendFailed,
	}

	for in, want := range tests {
		if got := NormalizeDeliveryErrorCode(in); got != want {
			t.Errorf("NormalizeDeliveryErrorCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeDeliveryError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{
			name: "known code is kept",
			in:   map[string]any{"code": DeliveryErrorFileTooLarge, "message": "too big"},
			want: map[string]any{"code": DeliveryErrorFileTooLarge, "message": "too big"},
		},
		{
			name: "raw provider code is normalized and kept aside",
			in:   map[string]any{"code": "131047", "message": "Re-engagement message", "details": "x"},
			want: map[string]any{"code": DeliveryErrorSendFailed, "provider_code": "131047", "message": "Re-engagement message", "details": "x"},
		},
		{
			name: "numeric code",
			in:   map[string]any{"code": float64(403)},
			want: map[string]any{"code": DeliveryErrorSendFailed, "provider_code": "403"},
		},
		{
			name: "no details at all",
			in:   nil,
			want: map[string]any{"code": DeliveryErrorSendFailed},
		},
		{
			name: "message is redacted",
			in:   map[string]any{"code": DeliveryErrorSendFailed, "message": "Get https://user:test-password@example.com/x: EOF"},
			want: map[string]any{"code": DeliveryErrorSendFailed, "message": "Get https://[REDACTED]@example.com/x: EOF"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := SanitizeDeliveryError(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SanitizeDeliveryError(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedactSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "telegram bot token in the path",
			in:   `Post "https://api.telegram.org/bot1234567:test-token-abcdefghijklmnopqrstuvwxyz/sendDocument": dial tcp: i/o timeout`,
			want: `Post "https://api.telegram.org/bot[REDACTED]/sendDocument": dial tcp: i/o timeout`,
		},
		{
			name: "bare telegram-style token",
			in:   "token 1234567:test-token-abcdefghijklmnopqrstuvwxyz rejected",
			want: "token [REDACTED] rejected",
		},
		{
			name: "userinfo in a url",
			in:   "dial https://bot:test-secret@example.com:8443/hook failed",
			want: "dial https://[REDACTED]@example.com:8443/hook failed",
		},
		{
			name: "token query parameter",
			in:   "GET https://graph.example.com/v19.0/me?fields=id&access_token=test-token-graph&x=1: 400",
			want: "GET https://graph.example.com/v19.0/me?fields=id&access_token=[REDACTED]&x=1: 400",
		},
		{
			name: "authorization header",
			in:   "request with Authorization: Bearer test-token-bearer.abc failed",
			want: "request with Authorization: Bearer [REDACTED] failed",
		},
		{
			name: "plain provider text is untouched",
			in:   "viber receiver not subscribed or unreachable (https://example.com/docs/errors#6)",
			want: "viber receiver not subscribed or unreachable (https://example.com/docs/errors#6)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := RedactSecrets(tc.in); got != tc.want {
				t.Errorf("RedactSecrets(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMessageTypeRetryable(t *testing.T) {
	t.Parallel()

	want := map[MessageType]bool{
		MessageTypeUnknown:     false,
		MessageTypeText:        true,
		MessageTypeFile:        true,
		MessageTypeImage:       false,
		MessageTypeSystem:      false,
		MessageTypeInteractive: false,
		MessageTypeLocation:    true,
		MessageTypeContact:     true,
	}

	for typ, retryable := range want {
		if got := typ.Retryable(); got != retryable {
			t.Errorf("%s.Retryable() = %v, want %v", typ, got, retryable)
		}
	}
}
