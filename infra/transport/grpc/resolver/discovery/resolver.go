package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/webitel/im-thread-service/infra/transport/endpoint"
	"github.com/webitel/im-thread-service/infra/transport/subset"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

var (
	_ resolver.Resolver = (*discoveryResolver)(nil)
)

type (
	discoveryResolver struct {
		w  discovery.Watcher
		cc resolver.ClientConn

		ctx    context.Context
		cancel context.CancelFunc

		insecure    bool
		debugLog    bool
		selectorKey string
		subsetSize  int
	}
)

// Close implements [resolver.Resolver].
func (r *discoveryResolver) Close() {
	r.cancel()

	if err := r.w.Stop(); err != nil {
		slog.Error("[RESOLVER] failed to watch top", "err", err)
	}
}

// ResolveNow implements [resolver.Resolver].
func (r *discoveryResolver) ResolveNow(_ resolver.ResolveNowOptions) {}

func (r *discoveryResolver) watch() {
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		ins, err := r.w.Next()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}

			slog.Error("[RESOLVER] failed to watch discovery endpoint", "err", err)
			time.Sleep(time.Second)

			continue
		}

		r.update(ins)
	}
}

func (r *discoveryResolver) update(ins []*discovery.ServiceInstance) {
	_, filtered := r.filterEndpoints(ins)

	if r.subsetSize != 0 {
		filtered = subset.Subset(r.selectorKey, filtered, r.subsetSize)
	}

	addrs := r.mapToAddresses(filtered)
	if len(addrs) == 0 {
		slog.Warn("[RESOLVER] zero endpoint found, refused to write, instances", "instances", ins)
	}

	if err := r.cc.UpdateState(resolver.State{Addresses: addrs}); err != nil {
		slog.Error("[RESOLVER] failed to update state", "err", err)
	}

	if r.debugLog {
		b, _ := json.Marshal(filtered)
		slog.Info("[RESOLVER] update instances", "service", b)
	}
}

func (r *discoveryResolver) filterEndpoints(ins []*discovery.ServiceInstance) (map[string]struct{}, []*discovery.ServiceInstance) {
	var (
		endpoints = make(map[string]struct{})
		filtered  = make([]*discovery.ServiceInstance, 0, len(ins))
	)

	for _, in := range ins {
		ept, err := endpoint.ParseEndpoint(in.Endpoints, endpoint.Scheme("grpc", !r.insecure))
		if err != nil {
			slog.Error("[RESOLVER] failed to parse discovery endpoint", "err", err)
			continue
		}

		if ept == "" {
			continue
		}

		if _, ok := endpoints[ept]; ok {
			continue
		}

		endpoints[ept] = struct{}{}
		filtered = append(filtered, in)
	}

	return endpoints, filtered
}

func (r *discoveryResolver) mapToAddresses(ins []*discovery.ServiceInstance) []resolver.Address {
	var (
		addrs = make([]resolver.Address, 0, len(ins))
	)

	for _, in := range ins {
		ept, _ := endpoint.ParseEndpoint(in.Endpoints, endpoint.Scheme("grpc", !r.insecure))
		addr := resolver.Address{
			ServerName: in.Name,
			Addr:       ept,
			Attributes: parseAttributes(in.Metadata).WithValue("rawServiceInstance", in),
		}
		addrs = append(addrs, addr)
	}

	return addrs
}

func parseAttributes(md map[string]string) (a *attributes.Attributes) {
	for k, v := range md {
		a = a.WithValue(k, v)
	}

	return a
}
