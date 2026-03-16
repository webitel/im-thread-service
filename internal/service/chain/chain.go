package chain

import (
	"context"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type ChainCallMember[T any, U any] func(ctx context.Context, r T) (U, error)

func ValidationWrapper[T model.Validator, U any](next ChainCallMember[T, U]) ChainCallMember[T, U] {
	return func(ctx context.Context, r T) (U, error) {
		if err := r.Validate(); err != nil {
			var zero U
			return zero, err
		}

		return next(ctx, r)
	}
}

type EnsureThreadCallback func(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error)

func EnsureThreadWrapper[T model.ThreadTarget, U any](ensureCallback EnsureThreadCallback) func(ChainCallMember[T, U]) ChainCallMember[T, U] {
    return func(next ChainCallMember[T, U]) ChainCallMember[T, U] {
        return func(ctx context.Context, r T) (U, error) {
			var (
				from = r.GetFrom()
				to = r.GetSendTo()
			)

            t, err := ensureCallback(ctx, &dto.EnsureDirectThreadRequest{
                DomainID: r.GetDomainID(),
                MemberID: r.GetFrom().ID,
                PeerFrom: &from,
                PeerTo:   &to,
            })
            if err != nil {
                var zero U
                return zero, err
            }
            r.SetThread(t.ID, t.Members)
            return next(ctx, r)
        }
    }
}

func Process[T any, U any](finalHandler ChainCallMember[T, U], wrappers ...func(ChainCallMember[T, U]) ChainCallMember[T, U]) ChainCallMember[T, U] {
    chain := finalHandler
    
    for i := len(wrappers) - 1; i >= 0; i-- {
        chain = wrappers[i](chain)
    }
    
    return chain
}

