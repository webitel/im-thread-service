//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

// Candidates, duplicates, the channel mark and the binding source round-trip.
//
//	POSTGRES_DSN=postgres://... go test -tags=integration -run ContactCandidate ./internal/store/postgres/ -count=1 -v
func TestContactCandidateStore_RoundTrip_Integration(t *testing.T) {
	tx := rolledBackTx(t)
	ctx := context.Background()
	th := newITThread(t, tx, uuid.New())
	store := NewContactCandidateStore(tx)
	now := time.Now().UTC().Truncate(time.Millisecond)

	_, checked, err := store.ChannelCheckedAt(ctx, th.id)
	require.NoError(t, err)
	require.False(t, checked)
	require.NoError(t, store.MarkChannelChecked(ctx, th.id, int(itDomainID), now))
	require.NoError(t, store.MarkChannelChecked(ctx, th.id, int(itDomainID), now), "idempotent")
	require.NoError(t, store.MarkChannelChecked(ctx, th.id, int(itDomainID), now.Add(-time.Minute)), "an older observation")
	checkedAt, checked, err := store.ChannelCheckedAt(ctx, th.id)
	require.NoError(t, err)
	require.True(t, checked)
	require.Equal(t, now, checkedAt.UTC(), "an older observation never replaces a newer one")
	require.NoError(t, store.MarkChannelChecked(ctx, th.id, int(itDomainID), now.Add(time.Minute)))
	checkedAt, _, err = store.ChannelCheckedAt(ctx, th.id)
	require.NoError(t, err)
	require.Equal(t, now.Add(time.Minute), checkedAt.UTC(), "a newer observation is stored")

	all, _ := model.MergeCandidates(nil, []model.ContactMatch{
		{CRMContactID: "2", Reason: model.MatchReason{Type: model.CriterionPhone, Trust: model.TrustUnverified, Origin: model.OriginFlow}},
		{CRMContactID: "1", Reason: model.MatchReason{Type: model.CriterionMessengerID, Trust: model.TrustVerified, Origin: model.OriginChannel}},
	}, nil, false, th.id, int(itDomainID), now)
	require.NoError(t, store.Upsert(ctx, all))
	require.NoError(t, store.Upsert(ctx, all), "same rows twice do not duplicate")
	require.NoError(t, store.Upsert(ctx, nil), "nothing to write")

	// A later run adds a reason to a known contact: the row is updated in place.
	_, changed := model.MergeCandidates(all, []model.ContactMatch{
		{CRMContactID: "2", Reason: model.MatchReason{Type: model.CriterionEmail, Trust: model.TrustUnverified, Origin: model.OriginFlow}},
	}, nil, false, th.id, int(itDomainID), now.Add(time.Second))
	require.NoError(t, store.Upsert(ctx, changed))

	got, err := store.List(ctx, th.id, int(itDomainID))
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "1", got[0].CRMContactID, "verified first")
	require.Equal(t, model.TrustVerified, got[0].Trust)
	require.Equal(t, model.CriterionMessengerID, got[0].Reasons[0].Type)
	require.Len(t, got[1].Reasons, 2, "the new reason is stored")
	require.Equal(t, now.Add(time.Second), got[1].UpdatedAt.UTC())

	other, err := store.List(ctx, th.id, int(itDomainID)+1)
	require.NoError(t, err)
	require.Empty(t, other, "another domain sees nothing")

	verifiedBy := []model.MatchReason{{Type: model.CriterionMessengerID, Trust: model.TrustVerified, Origin: model.OriginChannel}}
	dup := model.NewContactDuplicate(th.id, int(itDomainID), model.ContactCandidates{
		{CRMContactID: "3", Trust: model.TrustVerified, Reasons: verifiedBy}, {CRMContactID: "1", Trust: model.TrustVerified, Reasons: verifiedBy},
	})
	require.NoError(t, store.SaveDuplicate(ctx, dup))
	require.NoError(t, store.SaveDuplicate(ctx, dup), "a set is stored once")
	require.NoError(t, store.SaveDuplicate(ctx, nil), "nothing to write")

	var (
		dupIDs     []string
		dupReasons map[string][]model.MatchReason
		dupRows    int
	)

	require.NoError(t, tx.QueryRow(ctx, `
		select count(*) over (), contact_ids, reasons
		from im_thread.thread_contact_duplicate where thread_id = $1`, th.id).Scan(&dupRows, &dupIDs, &dupReasons))
	require.Equal(t, 1, dupRows)
	require.Equal(t, []string{"1", "3"}, dupIDs, "stored sorted")
	require.Equal(t, map[string][]model.MatchReason{"1": verifiedBy, "3": verifiedBy}, dupReasons)

	threads := NewThreadStore(tx)
	require.NoError(t, threads.SetCRMContact(ctx, th.id, "1", model.CRMContactSourceAuto))
	binding, err := threads.GetCRMContactBinding(ctx, th.id, int(itDomainID))
	require.NoError(t, err)
	require.Equal(t, model.CRMContactBinding{CRMContactID: "1", Source: model.CRMContactSourceAuto}, binding)

	require.NoError(t, threads.SetCRMContact(ctx, th.id, "", model.CRMContactSourceSelected))
	binding, err = threads.GetCRMContactBinding(ctx, th.id, int(itDomainID))
	require.NoError(t, err)
	require.Equal(t, model.CRMContactBinding{Source: model.CRMContactSourceSelected}, binding, "clearing keeps the operator's source")

	_, err = threads.GetCRMContactBinding(ctx, th.id, int(itDomainID)+1)
	require.Equal(t, codes.NotFound, errors.Code(err))
}
