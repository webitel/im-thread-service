package mapper

import (
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type ThreadInConverter struct{}

func (s *ThreadInConverter) ConvertGet(in *impb.GetThreadRequest) (*dto.ThreadGetRequest, error) {
	id, err := uuid.Parse(in.GetId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread id format", errors.WithCause(err))
	}

	return &dto.ThreadGetRequest{
		ID:       id,
		DomainID: int(in.GetDomainId()),
		Fields:   in.GetFields(),
	}, nil
}

func (s *ThreadInConverter) ConvertSearch(in *impb.ThreadSearchRequest) (*dto.ThreadSearchRequest, error) {
	ids, err := convertToUUIDs(in.GetIds())
	if err != nil {
		return nil, err
	}

	owners, err := convertToUUIDs(in.GetOwners())
	if err != nil {
		return nil, err
	}

	var domains []int
	for _, domain := range in.GetDomainIds() {
		domains = append(domains, int(domain))
	}

	members, err := convertToUUIDs(in.GetMemberIds())
	if err != nil {
		return nil, err
	}

	selfID, err := uuid.Parse(in.GetSelfId())
	if err != nil {
		return nil, err
	}

	return &dto.ThreadSearchRequest{
		Fields:     in.GetFields(),
		Q:          in.GetQ(),
		Sort:       in.GetSort(),
		Page:       int(in.GetPage()),
		Size:       int(in.GetSize()),
		IDs:        ids,
		DomainIDs:  domains,
		Kinds:      s.convertThreadKinds(in.GetKinds()),
		Owners:     owners,
		SelfID:     selfID,
		ContactIDs: members,
	}, nil
}

func (s *ThreadInConverter) convertThreadKinds(kinds []impb.ThreadKind) []model.ThreadKind {
	if kinds == nil {
		return nil
	}

	out := make([]model.ThreadKind, len(kinds))
	for i, kind := range kinds {
		out[i] = model.ThreadKind(kind)
	}

	return out
}

func (s *ThreadInConverter) ConvertSearchLeft(in *impb.SearchLeftRequest) (*dto.SearchLeftRequest, error) {
	memberID, err := uuid.Parse(in.GetMemberId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid member_id format", errors.WithCause(err))
	}

	return &dto.SearchLeftRequest{
		Fields:   in.GetFields(),
		MemberID: memberID,
		DomainID: int(in.GetDomainId()),
		Kinds:    s.convertThreadKinds(in.GetKinds()),
		Size:     int(in.GetSize()),
		Sort:     in.GetSort(),
		Page:     int(in.GetPage()),
	}, nil
}

func (s *ThreadInConverter) ConvertAddMemberRequest(in *impb.AddMemberRequest) (*dto.AddMemberRequest, error) {
	threadID, err := uuid.Parse(in.GetThreadId())
	if err != nil {
		return nil, err
	}

	newContactID, err := uuid.Parse(in.GetNewMemberContactId())
	if err != nil {
		return nil, err
	}

	converted := &dto.AddMemberRequest{
		ThreadID:           threadID,
		NewMemberContactID: newContactID,
		NewMemberRole:      s.convertMemberRole(in.GetRole()),
		DomainID:           int(in.GetDomainId()),
	}

	if in.InitiatorContactId != nil {
		initiatorContactID, err := uuid.Parse(in.GetInitiatorContactId())
		if err != nil {
			return nil, err
		}

		converted.InitiatorContactID = initiatorContactID
	}

	if in.AutoLeave != nil {
		v := in.GetAutoLeave()
		converted.AutoLeave = &v
	}

	return converted, nil
}

func (s *ThreadInConverter) ConvertTransferThreadRequest(in *impb.TransferRequest) (*dto.TransferThreadRequest, error) {
	threadID, err := uuid.Parse(in.GetThreadId())
	if err != nil {
		return nil, err
	}

	newContactID, err := uuid.Parse(in.GetNewMemberContactId())
	if err != nil {
		return nil, err
	}

	initiatorContactID, err := uuid.Parse(in.GetInitiatorContactId())
	if err != nil {
		return nil, err
	}

	converted := &dto.TransferThreadRequest{
		ThreadID:           threadID,
		NewMemberContactID: newContactID,
		NewMemberRole:      s.convertMemberRole(in.GetRole()),
		InitiatorContactID: initiatorContactID,
		TargetIsBot:        in.GetTargetIsBot(),
	}

	if in.AutoLeave != nil {
		v := in.GetAutoLeave()
		converted.AutoLeave = &v
	}

	return converted, nil
}

func (s *ThreadInConverter) ConvertRemoveMemberRequest(in *impb.RemoveMemberRequest) (*dto.RemoveMemberRequest, error) {
	targetMemberID, err := uuid.Parse(in.GetTargetMemberId())
	if err != nil {
		return nil, err
	}

	converted := &dto.RemoveMemberRequest{
		TargetMemberID: targetMemberID,
	}

	if in.Reason != nil {
		reason := in.GetReason()
		converted.Reason = &reason
	}

	if in.InitiatorContactId != nil {
		initiatorContactID, err := uuid.Parse(in.GetInitiatorContactId())
		if err != nil {
			return nil, err
		}

		converted.InitiatorContactID = initiatorContactID
	}

	return converted, nil
}

func (s *ThreadInConverter) convertMemberRole(in impb.ThreadRole) model.ThreadRole {
	switch in {
	case impb.ThreadRole_ROLE_OWNER:
		return model.RoleOwner
	case impb.ThreadRole_ROLE_ADMIN:
		return model.RoleAdmin
	case impb.ThreadRole_ROLE_MEMBER:
		return model.RoleMember
	case impb.ThreadRole_ROLE_SUPERVISOR:
		return model.RoleSupervisor
	case impb.ThreadRole_ROLE_UNSPECIFIED:
		return model.UnspecifiedRole
	default:
		return model.UnspecifiedRole
	}
}

type ThreadOutConverter struct{}

func (s *ThreadOutConverter) ConvertToThread(source *model.Thread) *impb.Thread {
	if source == nil {
		return nil
	}

	var lastMsg *impb.HistoryMessage

	if message := source.LastMessage; message != nil {
		msgMd, err := structpb.NewStruct(message.Metadata)
		if err != nil {
			return nil
		}

		lastMsg = &impb.HistoryMessage{
			Id:          message.ID.String(),
			ThreadId:    source.ID.String(),
			SenderId:    message.SenderID.String(),
			Type:        int32(message.Type),
			Body:        message.Body,
			Metadata:    msgMd,
			CreatedAt:   message.CreatedAtUnixMillis(),
			UpdatedAt:   message.UpdatedAtUnixMillis(),
			Documents:   mapDocs(message.Documents),
			Images:      mapImages(message.Images),
			Location:    mapLocation(message.Location),
			Contact:     mapContact(message.Contact),
			System:      mapSystem(message.System),
			Interactive: mapInteractive(message.Interactive),
		}
	}

	var vars *impb.ThreadVariables
	if source.Variables != nil {
		vars = MapThreadVariablesToProto(source.Variables)
	}

	thread := &impb.Thread{
		Id:          source.ID.String(),
		DomainId:    int32(source.DomainID),
		CreatedAt:   source.CreatedAtUnix(),
		UpdatedAt:   source.UpdatedAtUnix(),
		Kind:        impb.ThreadKind(source.Kind),
		Subject:     source.Subject,
		Description: source.Description,
		Members:     s.convertThreadMembers(source.Members, source.BotControllerID),
		LastMsg:     lastMsg,
		Variables:   vars,
	}

	if source.BotControllerID != nil {
		thread.BotControllerMemberId = source.BotControllerID.String()
	}

	if source.OwnerBotID != nil {
		thread.OwnerBotMemberId = source.OwnerBotID.String()
	}

	return thread
}

func (s *ThreadOutConverter) convertThreadMembers(members []*model.ThreadDialog, botControllerID *uuid.UUID) []*impb.ThreadMember {
	if members == nil {
		return nil
	}

	out := make([]*impb.ThreadMember, len(members))
	for i, member := range members {
		out[i] = &impb.ThreadMember{
			Id:                 member.ID.String(),
			ContactId:          member.ContactID.String(),
			Role:               s.ConvertThreadRole(member.ThreadRole),
			IsBot:              member.IsBot,
			AutoLeave:          member.AutoLeave,
			IsActiveController: botControllerID != nil && member.ID == *botControllerID,
		}
	}

	return out
}

func (s *ThreadOutConverter) ConvertThreadRole(in model.ThreadRole) impb.ThreadRole {
	switch in {
	case model.RoleOwner:
		return impb.ThreadRole_ROLE_OWNER
	case model.RoleAdmin:
		return impb.ThreadRole_ROLE_ADMIN
	case model.RoleMember:
		return impb.ThreadRole_ROLE_MEMBER
	case model.RoleSupervisor:
		return impb.ThreadRole_ROLE_SUPERVISOR
	case model.UnspecifiedRole:
		return impb.ThreadRole_ROLE_UNSPECIFIED
	default:
		return impb.ThreadRole_ROLE_UNSPECIFIED
	}
}

func MapSetVariablesRequestToCommand(req *impb.SetVariablesRequest) (*model.SetThreadVariablesCommand, error) {
	errorsIDWrapper := errors.WithID("grpc.thread.SetVariables")

	threadID, err := uuid.Parse(req.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid originator uuid format", errors.WithCause(err), errorsIDWrapper)
	}

	originator, err := uuid.Parse(req.GetMemberId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid originator uuid format", errors.WithCause(err), errorsIDWrapper)
	}

	vars := make(map[string]model.VariableEntry)
	for _, v := range req.GetVariables() {
		vars[v.GetKey()] = model.VariableEntry{
			Value: v.GetValue().AsMap(),
			SetBy: originator,
		}
	}

	return &model.SetThreadVariablesCommand{
		Member: originator,
		Variables: &model.ThreadVariables{
			ThreadID:  threadID,
			Variables: vars,
		},
	}, nil
}

func MapThreadVariablesToProto(vars *model.ThreadVariables) *impb.ThreadVariables {
	if vars == nil {
		return nil
	}

	protoVars := make(map[string]*impb.VariableEntry, len(vars.Variables))
	for k, v := range vars.Variables {
		value, err := structpb.NewStruct(v.Value)
		if err != nil {
			continue
		}

		protoVars[k] = &impb.VariableEntry{
			Value: value,
			SetBy: v.SetBy.String(),
			SetAt: v.SetAt.UTC().UnixMilli(),
		}
	}

	return &impb.ThreadVariables{
		ThreadId:  vars.ThreadID.String(),
		Variables: protoVars,
	}
}

func MapSearchVariablesRequestToQuery(req *impb.SearchVariablesRequest) (model.GetThreadVariablesQuery, error) {
	var threadIDs uuid.UUIDs
	if len(req.GetThreadIds()) > 0 {
		threadIDs = make([]uuid.UUID, len(req.GetThreadIds()))
		for i, id := range req.GetThreadIds() {
			var err error

			threadIDs[i], err = uuid.Parse(id)
			if err != nil {
				return model.GetThreadVariablesQuery{}, errors.InvalidArgument("invalid thread id format", errors.WithCause(err))
			}
		}
	}

	return model.GetThreadVariablesQuery{
		Pagination: model.Pagination{
			Limit: int(req.GetSize()),
			Page:  int(req.GetPage()),
		},
		Fields:    req.GetFields(),
		ThreadIDs: threadIDs,
	}, nil
}

func MapThreadVariablesPageToProto(page *model.Page[*model.ThreadVariables]) *impb.SearchVariablesResponse {
	items := make([]*impb.ThreadVariables, len(page.Items))
	for i, v := range page.Items {
		items[i] = MapThreadVariablesToProto(v)
	}

	return &impb.SearchVariablesResponse{
		Items: items,
		Next:  page.HasNext,
	}
}

func MapFlushVariablesRequestToCommand(req *impb.FlushVariablesRequest) (*model.FlushVariablesCommand, error) {
	threadID, err := uuid.Parse(req.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread id format", errors.WithCause(err))
	}

	originator, err := uuid.Parse(req.GetMemberId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid originator id format", errors.WithCause(err))
	}

	return &model.FlushVariablesCommand{
		ThreadID: threadID,
		Member:   originator,
		Keys:     req.GetKeys(),
	}, nil
}
