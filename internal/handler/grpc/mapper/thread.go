package mapper

import (
	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
	"google.golang.org/protobuf/types/known/structpb"
)

func MapThreadSearchRequestToDTO(request *impb.ThreadSearchRequest) *dto.SearchThreadRequest {
	var (
		ids     = utils.Map(request.Ids, utils.IdsParser)
		owners  = utils.Map(request.Owners, utils.IdsParser)
		members = utils.Map(request.MemberIds, utils.IdsParser)
		kinds   = utils.Map(request.Kinds, func(tk impb.ThreadKind) model.ThreadKind { return model.ThreadKind(tk) })
	)

	return &dto.SearchThreadRequest{
		Fields:    request.Fields,
		Ids:       ids,
		DomainIds: utils.ConvertInts[int](request.DomainIds),
		Kinds:     kinds,
		Owners:    owners,
		Q:         request.Q,
		MemberIds: members,
		Limit:     int(request.Size),
		Sort:      request.Sort,
		Page:      int(request.Page),
	}
}

func MapThreadsToProtoThreadList(threads []*model.Thread) *impb.SearchThreadResponse {
	protoThreads := utils.Map(threads, MapThreadModelToProtoSearchResponse)

	return &impb.SearchThreadResponse{
		Items: protoThreads,
	}
}

func MapThreadModelToProtoSearchResponse(thread *model.Thread) *impb.Thread {
	if thread == nil {
		return nil
	}

	var (
		memberIds = utils.Map(thread.MembersIDs, func(u uuid.UUID) string { return u.String() })
		members   = utils.Map(thread.Members, mapThreadMemberToProtoResponse)
	)

	var lstMsg *impb.HistoryMessage
	if thread.LastMessage != nil {
		lstMsgMD, _ := structpb.NewStruct(thread.LastMessage.Metadata)

		lstMsg = &impb.HistoryMessage{
			Id:        thread.LastMessage.ID.String(),
			SenderId:  thread.LastMessage.SenderID.String(),
			Type:      int32(thread.LastMessage.Type),
			Body:      thread.LastMessage.Body,
			Metadata:  lstMsgMD,
			CreatedAt: thread.LastMessage.CreatedAt.UTC().UnixMilli(),
			UpdatedAt: thread.LastMessage.UpdatedAt.UTC().UnixMilli(),
			Documents: mapDocs(thread.LastMessage.Documents),
			Images:    mapImages(thread.LastMessage.Images),
		}
	}

	return &impb.Thread{
		Id:          thread.ID.String(),
		DomainId:    int32(thread.DomainID),
		CreatedAt:   thread.CreatedAtUnix(),
		UpdatedAt:   thread.UpdatedAtUnix(),
		Kind:        impb.ThreadKind(thread.Kind),
		Owner:       thread.OwnerID.String(),
		Subject:     thread.Subject,
		Description: thread.Description,
		MemberIds:   memberIds,
		Members:     members,
		LastMsg:     lstMsg,
	}
}

func mapThreadMemberToProtoResponse(member *model.ThreadMember) *impb.ThreadMember {
	if member == nil {
		return nil
	}

	var (
		directSettings *impb.ThreadDirectSettings
	)

	if member.DirectSettings != nil {
		directSettings = &impb.ThreadDirectSettings{
			Id:        member.DirectSettings.ID.String(),
			DomainId:  int32(member.DirectSettings.DomainID),			
			UpdatedAt: member.DirectSettings.UpdatedAt.UTC().UnixMilli(),
			Title:     member.DirectSettings.Title,
		}
	}

	return &impb.ThreadMember{
		Id:             member.Id.String(),
		DirectSettings: directSettings,
	}
}
