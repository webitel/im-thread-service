package mapper

import (
	"time"

	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

//go:generate goverter gen .

// goverter:converter
// goverter:output:file ./generated/thread_to_dto.go
// goverter:matchIgnoreCase
// goverter:extend github.com/google/uuid:Parse
// goverter:extend ConvertInt32ToInt
// goverter:extend ConvertThreadKindToInternal
type ThreadInConverter interface {
	ConvertCreateGroup(*impb.CreateGroupRequest) *dto.CreateGroupRequest
	// goverter:map Size Limit
	ConvertSearch(*impb.ThreadSearchRequest) (*dto.ThreadSearchRequest, error)
}

// goverter:converter
// goverter:matchIgnoreCase
// goverter:output:file ./generated/thread_to_pb.go
// goverter:ignoreUnexported
// goverter:extend ConvertUUIDToString
// goverter:extend ConvertIntToInt32
// goverter:extend ConvertTimeToInt64
// goverter:extend ConvertThreadKindToExternal
// goverter:extend ConvertThreadMemberToProto
type ThreadOutConverter interface {
	ConvertCreateGroup(*dto.CreateGroupRequest) *impb.CreateGroupRequest
	// goverter:autoMap BaseModel
	// goverter:map MembersIds MemberIds
	// goverter:ignore Admins
	ConvertToThread(*model.Thread) *impb.Thread
}

func ConvertInt32ToInt(num int32) int {
	return int(num)
}

func ConvertIntToInt32(num int) int32 {
	return int32(num)
}

func ConvertUUIDToString(id uuid.UUID) string {
	return id.String()
}

func ConvertTimeToInt64(in time.Time) int64 {
	return in.UnixMilli()
}

func ConvertThreadKindToInternal(in impb.ThreadKind) model.ThreadKind {
	return model.ThreadKind(in)
}

func ConvertThreadKindToExternal(in model.ThreadKind) impb.ThreadKind {
	return impb.ThreadKind(in)
}

func ConvertMemberToID(in *model.ThreadMember) string {
	if in == nil {
		return ""
	}
	return in.Id.String()
}

func ConvertThreadMemberToProto(member *model.ThreadMember) *impb.ThreadMember {
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
			CreatedAt: member.DirectSettings.CreatedAtMilliseconds(),
			UpdatedAt: member.DirectSettings.UpdatedAtMilliseconds(),
			Title:     member.DirectSettings.Title,
		}
	}

	return &impb.ThreadMember{
		Id:             member.Id.String(),
		DirectSettings: directSettings,
	}
}
