package grpc

import (
	"context"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
)

// ContactIdentifier is the service behind the ContactIdentification API.
type ContactIdentifier interface {
	Identify(ctx context.Context, cmd *model.IdentifyContactCommand) (*model.IdentifyContactResult, error)
	ListCandidates(ctx context.Context, threadID uuid.UUID, domainID int) (*service.ContactCandidatesView, error)
}

type ContactIdentificationServer struct {
	impb.UnimplementedContactIdentificationServer

	identifier ContactIdentifier
}

func NewContactIdentificationServer(identifier ContactIdentifier) *ContactIdentificationServer {
	return &ContactIdentificationServer{identifier: identifier}
}

func RegisterContactIdentificationServer(srv *grpcsrv.Server, svc *ContactIdentificationServer) {
	impb.RegisterContactIdentificationServer(srv.Server, svc)
}

func (s *ContactIdentificationServer) Identify(ctx context.Context, req *impb.IdentifyContactRequest) (*impb.IdentifyContactResponse, error) {
	cmd, err := mapper.MapIdentifyContactRequestToCommand(req)
	if err != nil {
		return nil, err
	}

	result, err := s.identifier.Identify(ctx, cmd)
	if err != nil {
		return nil, err
	}

	return mapper.MapIdentifyContactResultToProto(result), nil
}

func (s *ContactIdentificationServer) ListCandidates(ctx context.Context, req *impb.ListContactCandidatesRequest) (*impb.ListContactCandidatesResponse, error) {
	threadID, err := uuid.Parse(req.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread_id", errors.WithCause(err))
	}

	view, err := s.identifier.ListCandidates(ctx, threadID, int(req.GetDomainId()))
	if err != nil {
		return nil, err
	}

	return mapper.MapContactCandidatesToProto(view.Candidates, view.Binding), nil
}
