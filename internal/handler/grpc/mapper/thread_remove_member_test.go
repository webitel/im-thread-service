package mapper

import (
	"testing"

	"github.com/stretchr/testify/require"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
)

func TestThreadInConverter_ConvertRemoveMemberRequest_MapsReason(t *testing.T) {
	target := "f3c43767-2d68-4df7-b6f8-cd57ec050dc1"
	initiator := "f1007f00-a8e7-4af1-a59b-2b8c8c17ce95"
	reason := "left voluntarily"

	in := &impb.RemoveMemberRequest{
		TargetMemberId:     target,
		InitiatorContactId: &initiator,
		Reason:             &reason,
	}

	out, err := (&ThreadInConverter{}).ConvertRemoveMemberRequest(in)
	require.NoError(t, err)
	require.NotNil(t, out.Reason)
	require.Equal(t, reason, *out.Reason)
	require.Equal(t, target, out.TargetMemberID.String())
	require.Equal(t, initiator, out.InitiatorContactID.String())
}

func TestThreadInConverter_ConvertRemoveMemberRequest_ReasonNilWhenOmitted(t *testing.T) {
	target := "f3c43767-2d68-4df7-b6f8-cd57ec050dc1"

	in := &impb.RemoveMemberRequest{
		TargetMemberId: target,
	}

	out, err := (&ThreadInConverter{}).ConvertRemoveMemberRequest(in)
	require.NoError(t, err)
	require.Nil(t, out.Reason)
}
