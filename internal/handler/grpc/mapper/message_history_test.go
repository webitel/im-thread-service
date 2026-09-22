package mapper

import (
	"testing"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
)

// TestSystemMessageAllowListTypes covers the 3-state presence semantics this
// whole feature depends on: an unset SystemMessageAllowList field must map to
// nil (not restricted), while a present field — even with an empty/nil Types
// list — must map to a non-nil slice (restricted). Collapsing "unset" and
// "present but empty" into the same nil value here would silently turn
// "block all system messages" into "not restricted", which is exactly the
// regression this test guards against.
func TestSystemMessageAllowListTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		allowList *impb.SystemMessageAllowList
		wantNil   bool
		want      []string
	}{
		{
			name:      "unset field is not restricted",
			allowList: nil,
			wantNil:   true,
		},
		{
			name:      "present with nil Types blocks all system messages",
			allowList: &impb.SystemMessageAllowList{},
			want:      []string{},
		},
		{
			name:      "present with empty Types blocks all system messages",
			allowList: &impb.SystemMessageAllowList{Types: []string{}},
			want:      []string{},
		},
		{
			name:      "present with a non-empty Types list restricts to those subtypes",
			allowList: &impb.SystemMessageAllowList{Types: []string{"user_joined", "user_left"}},
			want:      []string{"user_joined", "user_left"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := systemMessageAllowListTypes(tt.allowList)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("want nil (not restricted), got %#v", got)
				}

				return
			}

			if got == nil {
				t.Fatalf("want non-nil (restricted), got nil")
			}

			if len(got) != len(tt.want) {
				t.Fatalf("want %#v, got %#v", tt.want, got)
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("want %#v, got %#v", tt.want, got)
				}
			}
		})
	}
}

// TestMapSearchMessageHistoryRequest2HistoryMessageInputDTO_SystemMessageAllowList
// asserts the same 3-state semantics survive the full request mapper, not
// just the helper function in isolation.
func TestMapSearchMessageHistoryRequest2HistoryMessageInputDTO_SystemMessageAllowList(t *testing.T) {
	t.Parallel()

	unset := MapSearchMessageHistoryRequest2HistoryMessageInputDTO(&impb.SearchMessageHistoryRequest{})
	if unset.SystemMessageAllowListTypes != nil {
		t.Fatalf("unset field must map to nil (not restricted), got %#v", unset.SystemMessageAllowListTypes)
	}

	blockAll := MapSearchMessageHistoryRequest2HistoryMessageInputDTO(&impb.SearchMessageHistoryRequest{
		SystemMessageAllowList: &impb.SystemMessageAllowList{},
	})
	if blockAll.SystemMessageAllowListTypes == nil {
		t.Fatalf("present-but-empty field must map to a non-nil slice (restricted, block-all)")
	}

	if len(blockAll.SystemMessageAllowListTypes) != 0 {
		t.Fatalf("expected empty allow-list, got %#v", blockAll.SystemMessageAllowListTypes)
	}
}
