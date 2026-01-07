package guards

import (
	"errors"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

//#region EnsureDirectThread

func EnsureDirectThreadValidationGuard(request *dto.EnsureDirectThreadRequest) error {
	guards := []ValidationGuard[*dto.EnsureDirectThreadRequest]{
		nilRequest(), // CRITICAL TO CHECK FOR NON NIL FIRST!
		nilPeers(),
		emptyDomain(),
		selfSend(),
		emptyMember(),
		emptyPeers(),
	}

	for _, guard := range guards {
		if err := guard(request); err != nil {
			return err // QUICK LEAVE AFTER FIRST INVALID VALIDATION!
		}
	}

	return nil
}

func nilRequest() ValidationGuard[*dto.EnsureDirectThreadRequest] {
	return func(req *dto.EnsureDirectThreadRequest) error {
		if req == nil {
			return errors.New("request is nil!")
		}

		return nil
	}
}

func nilPeers() ValidationGuard[*dto.EnsureDirectThreadRequest] {
	return func(req *dto.EnsureDirectThreadRequest) error {
		if req.PeerFrom == nil || req.PeerTo == nil {
			return errors.New("both peers must be not nil equal!")
		}

		return nil
	}
}

func emptyDomain() ValidationGuard[*dto.EnsureDirectThreadRequest] {
	return func(req *dto.EnsureDirectThreadRequest) error {
		if req.DomainID <= 0 {
			return errors.New("domain id required!")
		}

		return nil
	}
}

func selfSend() ValidationGuard[*dto.EnsureDirectThreadRequest] {
	return func(req *dto.EnsureDirectThreadRequest) error {
		if req.PeerFrom.ID == req.PeerTo.ID {
			return errors.New("can not create direct chat with yourself, forbidden!")
		}

		return nil
	}
}

func emptyMember() ValidationGuard[*dto.EnsureDirectThreadRequest] {
	return func(req *dto.EnsureDirectThreadRequest) error {
		if req.MemberID == uuid.Nil {
			return errors.New("member is empty!")
		}

		return nil
	}
}

func emptyPeers() ValidationGuard[*dto.EnsureDirectThreadRequest] {
	return func(req *dto.EnsureDirectThreadRequest) error {
		if req.PeerFrom.ID == uuid.Nil || req.PeerTo.ID == uuid.Nil {
			return errors.New("some of peers is empty!")
		}

		return nil
	}
}

func CanSendRightsViolationGuard(canSend bool) error {
	if !canSend {
		return errors.New("send message rights violation!")
	}

	return nil
}

//#endregion
