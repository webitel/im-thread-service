package leader

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/webitel/webitel-go-kit/infra/discovery"

	"github.com/webitel/im-thread-service/config"
)

type LeadershipElector interface {
	Run(ctx context.Context, onStart func(ctx context.Context) error, onStop func())
}

// [CONSUL_KEYS]
const leaderElectionKey = "service/im-thread-service/leader"

type LeaderElector struct {
	client *api.Client
	log    *slog.Logger
	key    string
	nodeID string

	ttl           string // Consul session TTL, formatted (e.g. "10s")
	renewInterval time.Duration
	errCooldown   time.Duration
	blockingWait  time.Duration
	lockDelay     time.Duration
}

func NewLeaderElector(consulAddr, nodeID string, cfg config.LeaderElectionConfig, log *slog.Logger) (*LeaderElector, error) {
	consulCfg := api.DefaultConfig()
	consulCfg.Address = consulAddr

	client, err := api.NewClient(consulCfg)
	if err != nil {
		return nil, fmt.Errorf("consul client init failed: %w", err)
	}

	return &LeaderElector{
		client:        client,
		log:           log.With("component", "leader-elector", "key", leaderElectionKey),
		key:           leaderElectionKey,
		nodeID:        nodeID,
		ttl:           cfg.TTL.String(),
		renewInterval: cfg.RenewInterval,
		errCooldown:   cfg.ErrCooldown,
		blockingWait:  cfg.BlockingWait,
		lockDelay:     cfg.LockDelay,
	}, nil
}

func ProvideLeaderElector(cfg *config.Config, log *slog.Logger) (*LeaderElector, error) {
	return NewLeaderElector(
		cfg.Consul.Addr,
		discovery.GenerateInstanceID("im-thread-service"),
		cfg.LeaderElection,
		log,
	)
}

// Run blocks and continuously tries to acquire leadership
func (le *LeaderElector) Run(ctx context.Context, onStart func(ctx context.Context) error, onStop func()) {
	le.log.Info("leader election loop started", "node_id", le.nodeID, "ttl", le.ttl, "renew_interval", le.renewInterval)

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
		le.log.Error("failed to create session", "err", err)
		le.wait(ctx, le.errCooldown)

		return
	}

	le.log.Info("consul session created, attempting to acquire lock", "session", sessionID)

	defer le.destroySession(sessionID)

	// 2. [ACQUIRE_LOCK]
	// Try to write our NodeID to the leader key using our Session
	acquired, lastIndex, err := le.acquireLock(sessionID)
	if err != nil {
		le.log.Error("error during lock acquisition", "err", err)
		le.wait(ctx, le.errCooldown)

		return
	}

	if !acquired {
		le.log.Info("leader lock held by another instance, watching for release", "index", lastIndex)
		// [BLOCKING_WATCH] Block until the key actually changes instead of polling on a timer
		le.watchKey(ctx, lastIndex)
		le.log.Info("blocking watch returned, retrying acquisition")

		return
	}

	le.log.Info("node promoted to leader", "node_id", le.nodeID, "session", sessionID)

	// [LEADER_CONTEXT]
	// Bound to the duration of our leadership
	leaderCtx, cancelLeader := context.WithCancel(ctx)
	defer cancelLeader()

	// [RENEW_STOP] Deliberately NOT tied to leaderCtx: consul/api's RenewPeriodic calls
	// Session().Destroy() itself as soon as this channel closes, which Consul treats as
	// invalidation and triggers LockDelay. leaderCtx gets canceled the instant the parent ctx
	// does (e.g. on shutdown), which would race our own explicit releaseLock() below — whichever
	// wins nondeterministically decides if LockDelay applies. Keeping this channel separate lets
	// us release the lock ourselves first, and only then let the renewal goroutine clean up.
	renewDone := make(chan struct{})

	// Keep session alive via background heartbeat
	go func() {
		if err := le.client.Session().RenewPeriodic(le.renewInterval.String(), sessionID, nil, renewDone); err != nil {
			le.log.Error("consul session renewal failed, stepping down", "err", err)
			cancelLeader()
		}
	}()

	// [START_WORKER]
	// Execute leader-only tasks (e.g., Outbox Forwarder)
	go func() {
		if err := onStart(leaderCtx); err != nil {
			le.log.Error("leader task execution failed", "err", err)
			cancelLeader()
		}
	}()

	// [WATCHDOG]
	// Monitor if we are still the leader in Consul KV storage
	le.monitorLeadership(leaderCtx, sessionID)

	le.log.Warn("node demoted: releasing leadership")
	onStop()

	// [GRACEFUL_RELEASE] Release the KV lock ourselves first — this does NOT trigger LockDelay,
	// unlike session invalidation/destroy — then let the renewal goroutine stop and clean up
	// the now-unused session.
	le.releaseLock(sessionID)
	close(renewDone)
}

func (le *LeaderElector) createSession() (string, error) {
	entry := &api.SessionEntry{
		Name:     "im-thread-leader-lock",
		TTL:      le.ttl,
		Behavior: api.SessionBehaviorRelease, // Release the lock if session expires
		// [LOCK_DELAY] Per Consul's session API, this is an anti-flapping grace period enforced
		// after any lock release (explicit or via session invalidation) and must be > 0 — Consul
		// falls back to its own 15s default otherwise. Kept low (configured, not Consul's 15s
		// default) since our forwarder is idempotent (offset-tracked in Postgres).
		LockDelay: le.lockDelay,
	}
	sessionID, _, err := le.client.Session().Create(entry, nil)

	return sessionID, err
}

// acquireLock tries to take the lock and returns the key's current ModifyIndex,
// used as the baseline for a blocking watch when acquisition fails.
func (le *LeaderElector) acquireLock(sessionID string) (bool, uint64, error) {
	kv := &api.KVPair{
		Key:     le.key,
		Value:   []byte(le.nodeID),
		Session: sessionID,
	}

	acquired, _, err := le.client.KV().Acquire(kv, nil)
	if err != nil {
		return false, 0, err
	}

	if acquired {
		return true, 0, nil
	}

	// [CURRENT_INDEX] Acquire's WriteMeta doesn't carry the key's index on failure,
	// so fetch it explicitly to use as the watch baseline.
	_, getMeta, getErr := le.client.KV().Get(le.key, nil)
	if getErr != nil || getMeta == nil {
		return false, 0, nil
	}

	return false, getMeta.LastIndex, nil
}

// watchKey blocks until the leader key changes (lock released/stolen) or ctx is canceled.
func (le *LeaderElector) watchKey(ctx context.Context, lastIndex uint64) {
	q := (&api.QueryOptions{WaitIndex: lastIndex, WaitTime: le.blockingWait}).WithContext(ctx)

	_, _, err := le.client.KV().Get(le.key, q)
	if err != nil && ctx.Err() == nil {
		le.log.Debug("blocking watch failed, falling back to cooldown", "err", err)
		le.wait(ctx, le.errCooldown)
	}
}

// monitorLeadership blocks on the leader key until it changes, instead of polling on a ticker.
func (le *LeaderElector) monitorLeadership(ctx context.Context, sessionID string) {
	var lastIndex uint64

	for {
		if ctx.Err() != nil {
			return
		}

		q := (&api.QueryOptions{WaitIndex: lastIndex, WaitTime: le.blockingWait}).WithContext(ctx)

		pair, meta, err := le.client.KV().Get(le.key, q)

		if ctx.Err() != nil {
			return
		}

		if err != nil {
			le.log.Debug("leadership watch failed", "err", err)

			return
		}

		if meta != nil {
			lastIndex = meta.LastIndex
		}

		// Check if someone else took the lock or the session was invalidated
		if pair == nil || pair.Session != sessionID {
			le.log.Debug("leadership check failed or session changed")

			return
		}
	}
}

// releaseLock explicitly hands off the KV lock via Consul's release operation, which — unlike
// session invalidation — does not incur the LockDelay grace period.
func (le *LeaderElector) releaseLock(sessionID string) {
	kv := &api.KVPair{
		Key:     le.key,
		Session: sessionID,
	}

	released, _, err := le.client.KV().Release(kv, nil)
	if err != nil {
		le.log.Warn("failed to explicitly release lock, falling back to lock-delay/session cleanup", "err", err)

		return
	}

	le.log.Info("lock released explicitly", "session", sessionID, "released", released)
}

func (le *LeaderElector) destroySession(sessionID string) {
	if sessionID == "" {
		return
	}

	if _, err := le.client.Session().Destroy(sessionID, nil); err != nil {
		le.log.Warn("failed to destroy session", "session", sessionID, "err", err)
	}
}

func (le *LeaderElector) wait(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
