package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service"
)

func TestDirectThreadCreatorGuard_CheckPreconditions(t *testing.T) {
	guard := service.NewDirectThreadCreatorGuard()
	ctx := context.Background()

	validInitiator := shared.Peer{ID: uuid.New(), Type: shared.PeerContact}
	validPeer := shared.Peer{
		ID:       uuid.New(),
		Type:     shared.PeerContact,
		Identity: &shared.Identity{Issuer: "test", Name: "John"},
	}

	tests := []struct {
		name    string
		req     *service.CreateThreadRequest
		wantErr string
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: "received nil pointer call for create thread request",
		},
		{
			name: "invalid domain id",
			req: service.NewCreateThreadRequest(0, validInitiator,
				service.WithKind(model.ThreadDirect),
				service.WithDirectConfig(validPeer),
			),
			wantErr: "domain id is required",
		},
		{
			name: "initiator is nil uuid",
			req: service.NewCreateThreadRequest(1, shared.Peer{ID: uuid.Nil, Type: shared.PeerContact},
				service.WithKind(model.ThreadDirect),
				service.WithDirectConfig(validPeer),
			),
			wantErr: "initiator is required",
		},
		{
			name: "initiator type is not contact",
			req: service.NewCreateThreadRequest(1, shared.Peer{ID: uuid.New(), Type: shared.PeerGroup},
				service.WithKind(model.ThreadDirect),
				service.WithDirectConfig(validPeer),
			),
			wantErr: "initiator must be a contact",
		},
		{
			name: "missing payload / nil payload",
			req: &service.CreateThreadRequest{
				DomainID:  1,
				Initiator: validInitiator,
				Payload:   nil,
			},
			wantErr: "invalid payload type, expected direct config",
		},
		{
			name: "invalid payload type",
			req: &service.CreateThreadRequest{
				DomainID:  1,
				Initiator: validInitiator,
				Payload:   "some random string",
			},
			wantErr: "invalid payload type, expected direct config",
		},
		{
			name: "peer is nil uuid",
			req: service.NewCreateThreadRequest(1, validInitiator,
				service.WithKind(model.ThreadDirect),
				service.WithDirectConfig(shared.Peer{ID: uuid.Nil, Type: shared.PeerContact}),
			),
			wantErr: "received nil uuid for direct member",
		},
		{
			name: "self direct chat attempt forbidden",
			req: service.NewCreateThreadRequest(1, validInitiator,
				service.WithKind(model.ThreadDirect),
				service.WithDirectConfig(shared.Peer{
					ID:       validInitiator.ID,
					Type:     shared.PeerContact,
					Identity: &shared.Identity{Issuer: "test"},
				}),
			),
			wantErr: "initiator and peer cannot be the same contact",
		},
		{
			name: "valid request",
			req: service.NewCreateThreadRequest(1, validInitiator,
				service.WithKind(model.ThreadDirect),
				service.WithDirectConfig(validPeer),
			),
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := guard.CheckPreconditions(ctx, tt.req)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
