package grpc

import (
	"context"

	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

var _ impb.ThreadPreviewServiceServer = (*ThreadPreviewServer)(nil)

type ThreadPreviewService interface {
	GivePreview(ctx context.Context, req *dto.GivePreviewRequest) (*model.ThreadPreview, error)
	RemoveFromPreview(ctx context.Context, req *dto.RemoveFromPreviewRequest) error
	UpgradePreviewToFullMember(ctx context.Context, req *dto.UpgradePreviewRequest) (uuid.UUID, error)
}

type ThreadPreviewServer struct {
	impb.UnimplementedThreadPreviewServiceServer

	svc ThreadPreviewService
}

func NewThreadPreviewServer(svc ThreadPreviewService) *ThreadPreviewServer {
	return &ThreadPreviewServer{
		svc: svc,
	}
}

func (s *ThreadPreviewServer) GivePreview(ctx context.Context, req *impb.GivePreviewRequest) (*impb.GivePreviewResponse, error) {
	converted, err := mapper.ConvertGivePreviewRequest(req)
	if err != nil {
		return nil, err
	}

	preview, err := s.svc.GivePreview(ctx, converted)
	if err != nil {
		return nil, err
	}

	return &impb.GivePreviewResponse{
		Preview: mapper.ConvertToThreadPreview(preview),
	}, nil
}

func (s *ThreadPreviewServer) RemoveFromPreview(ctx context.Context, req *impb.RemoveFromPreviewRequest) (*impb.RemoveFromPreviewResponse, error) {
	converted, err := mapper.ConvertRemoveFromPreviewRequest(req)
	if err != nil {
		return nil, err
	}

	err = s.svc.RemoveFromPreview(ctx, converted)
	if err != nil {
		return nil, err
	}

	return &impb.RemoveFromPreviewResponse{}, nil
}

func (s *ThreadPreviewServer) UpgradePreviewToFullMember(ctx context.Context, req *impb.UpgradePreviewToFullMemberRequest) (*impb.UpgradePreviewToFullMemberResponse, error) {
	converted, err := mapper.ConvertUpgradePreviewRequest(req)
	if err != nil {
		return nil, err
	}

	memberID, err := s.svc.UpgradePreviewToFullMember(ctx, converted)
	if err != nil {
		return nil, err
	}

	return &impb.UpgradePreviewToFullMemberResponse{
		Member: &impb.ThreadMember{
			Id:        memberID.String(),
			ContactId: req.GetContactId(),
			Role:      req.GetRole(),
		},
	}, nil
}
