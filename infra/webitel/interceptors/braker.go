package interceptors

import (
	"context"
	"sync"
	"time"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type BreakerInterceptor struct {
	// breakers maps method names to their respective circuit breakers
	breakers sync.Map
}

func NewBreakerInterceptor() *BreakerInterceptor {
	return &BreakerInterceptor{}
}

// UnaryClientInterceptor returns a gRPC interceptor with circuit breaker logic
func (bi *BreakerInterceptor) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// [LAZY_LOAD] Get or initialize a breaker for the specific RPC method
		val, _ := bi.breakers.LoadOrStore(method, gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    method,
			Timeout: 30 * time.Second, // Time to stay in OPEN state
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				// Trip if 5 consecutive failures occur
				return counts.ConsecutiveFailures > 5
			},
		}))

		cb := val.(*gobreaker.CircuitBreaker)

		// [EXECUTE] Wrap the network call
		_, err := cb.Execute(func() (any, error) {
			invokerErr := invoker(ctx, method, req, reply, cc, opts...)
			if invokerErr != nil {
				st, ok := status.FromError(invokerErr)
				if ok {
					// Only trip for infrastructure errors (Server down, Network, Timeout)
					switch st.Code() {
					case codes.Internal, codes.Unavailable, codes.DeadlineExceeded:
						return nil, invokerErr
					}
				}
			}
			return nil, invokerErr
		})

		// [FALLBACK] Map breaker's OpenState to gRPC Unavailable
		if err != nil && err == gobreaker.ErrOpenState {
			return status.Error(codes.Unavailable, "circuit breaker is open for: "+method)
		}

		return err
	}
}
