package leader

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/webitel/webitel-go-kit/infra/discovery"
	"github.com/webitel/webitel-go-kit/pkg/semconv"

	"github.com/webitel/im-thread-service/config"
)

type LeadershipElector interface {
	Run(ctx context.Context, onStart func(ctx context.Context) error, onStop func())
}

const (
	// [CONSUL_KEYS]
	leaderElectionKey = "service/im-thread-service/leader"
	sessionTTL        = "15s"
	renewInterval     = "10s"
	retryInterval     = 10 * time.Second
	errCooldown       = 5 * time.Second
	monitorInterval   = 5 * time.Second
)

type LeaderElector struct {
	client *api.Client
	log    *slog.Logger
	key    string
	nodeID string
}

func NewLeaderElector(consulAddr, nodeID string, log *slog.Logger) (*LeaderElector, error) {
	cfg := api.DefaultConfig()
	cfg.Address = consulAddr

	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("consul client init failed: %w", err)
	}

	return &LeaderElector{
		client: client,
		log:    log.With(semconv.ComponentKey, "leader-elector", "key", leaderElectionKey),
		key:    leaderElectionKey,
		nodeID: nodeID,
	}, nil
}

func ProvideLeaderElector(cfg *config.Config, log *slog.Logger) (*LeaderElector, error) {
	return NewLeaderElector(cfg.Consul.Addr, discovery.GenerateInstanceID("im-thread-service"), log)
}

// Run blocks and continuously tries to acquire leadership
func (le *LeaderElector) Run(ctx context.Context, onStart func(ctx context.Context) error, onStop func()) {
	for {
		select {
		case <-ctx.Done():
			le.log.Info("stopping leader election: context canceled")

			return
		default:
			// [ELECTION_LOOP]
			// Keep trying to become leader until context is closed
			le.attemptLeadership(ctx, onStart, onStop)
		}
	}
}

func (le *LeaderElector) attemptLeadership(ctx context.Context, onStart func(ctx context.Context) error, onStop func()) {
	// 1. [SESSION_INIT]
	// Create a volatile session in Consul tied to TTL
	sessionID, err := le.createSession()
	if err != nil {
		le.log.Error("failed to create session", semconv.ErrorKey, err)
		le.wait(ctx, errCooldown)

		return
	}

	defer le.destroySession(sessionID)

	// 2. [ACQUIRE_LOCK]
	// Try to write our NodeID to the leader key using our Session
	acquired, err := le.acquireLock(sessionID)
	if err != nil {
		le.log.Error("error during lock acquisition", semconv.ErrorKey, err)
		le.wait(ctx, errCooldown)

		return
	}

	if !acquired {
		le.log.Debug("leader lock held by another instance")
		le.wait(ctx, retryInterval)

		return
	}

	le.log.Info("node promoted to leader", "node_id", le.nodeID, "session", sessionID)

	// [LEADER_CONTEXT]
	// Bound to the duration of our leadership
	leaderCtx, cancelLeader := context.WithCancel(ctx)
	defer cancelLeader()

	// Keep session alive via background heartbeat
	go func() {
		if err := le.client.Session().RenewPeriodic(renewInterval, sessionID, nil, leaderCtx.Done()); err != nil {
			le.log.Error("consul session renewal failed, stepping down", semconv.ErrorKey, err)
			cancelLeader()
		}
	}()

	// [START_WORKER]
	// Execute leader-only tasks (e.g., Outbox Forwarder)
	go func() {
		if err := onStart(leaderCtx); err != nil {
			le.log.Error("leader task execution failed", semconv.ErrorKey, err)
			cancelLeader()
		}
	}()

	// [WATCHDOG]
	// Monitor if we are still the leader in Consul KV storage
	le.monitorLeadership(leaderCtx, sessionID)

	le.log.Warn("node demoted: releasing leadership")
	onStop()
}

func (le *LeaderElector) createSession() (string, error) {
	entry := &api.SessionEntry{
		Name:     "im-thread-leader-lock",
		TTL:      sessionTTL,
		Behavior: api.SessionBehaviorRelease, // Release the lock if session expires
	}
	sessionID, _, err := le.client.Session().Create(entry, nil)

	return sessionID, err
}

func (le *LeaderElector) acquireLock(sessionID string) (bool, error) {
	kv := &api.KVPair{
		Key:     le.key,
		Value:   []byte(le.nodeID),
		Session: sessionID,
	}
	acquired, _, err := le.client.KV().Acquire(kv, nil)

	return acquired, err
}

func (le *LeaderElector) monitorLeadership(ctx context.Context, sessionID string) {
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if someone else took the lock or session was invalidated
			pair, _, err := le.client.KV().Get(le.key, nil)
			if err != nil || pair == nil || pair.Session != sessionID {
				le.log.Debug("leadership check failed or session changed")

				return
			}
		}
	}
}

func (le *LeaderElector) destroySession(sessionID string) {
	if sessionID != "" {
		_, _ = le.client.Session().Destroy(sessionID, nil)
	}
}

func (le *LeaderElector) wait(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
