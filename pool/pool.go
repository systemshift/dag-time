// Package pool manages event synchronization between nodes
package pool

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/systemshift/dag-time/beacon"
	"github.com/systemshift/dag-time/dag"
)

// Config represents pool configuration
type Config struct {
	// EventRate is how quickly to generate events (in milliseconds)
	EventRate int64

	// AnchorInterval is number of events before anchoring to drand beacon
	AnchorInterval int

	// SubEventComplex is probability of creating sub-events (0.0-1.0)
	SubEventComplex float64

	// VerifyInterval is events between verifications
	VerifyInterval int

	// Verbose enables detailed logging of event relationships.
	// When Logger is nil, Verbose=true installs a Debug-level stderr
	// handler. When Logger is non-nil, Verbose is ignored — control the
	// level on your own handler.
	Verbose bool

	// RNGSeed seeds the per-pool RNG. Zero means use time.Now().UnixNano().
	// Set explicitly for deterministic tests.
	RNGSeed int64

	// Logger is the structured logger used by the pool. Nil uses
	// slog.Default() (Verbose may bump it to Debug — see above).
	Logger *slog.Logger
}

// Subscription represents a registered subscriber to pool events.
// Callers MUST call Unsubscribe when done to release the subscription channel.
type Subscription interface {
	// Events returns the channel that receives events. The channel is
	// closed when the pool shuts down or Unsubscribe is called.
	Events() <-chan *dag.Event

	// Unsubscribe removes this subscription and closes the channel.
	// Safe to call multiple times.
	Unsubscribe()
}

// Pool manages a pool of events and handles event synchronization
type Pool interface {
	// AddEvent adds a new event to the pool and returns the event ID.
	// Returns ErrPoolClosed if the pool has been closed.
	AddEvent(ctx context.Context, data []byte, parents []string) (string, error)

	// Subscribe registers a new subscriber and returns a Subscription handle.
	// Slow subscribers will lose events: when the per-subscriber buffer
	// (size 100) is full, notifications for that subscriber are dropped
	// silently. Use DroppedNotifications to observe the drop count.
	Subscribe() (Subscription, error)

	// Errors returns a channel of asynchronous errors from the pool
	Errors() <-chan error

	// DroppedNotifications returns the cumulative count of subscriber
	// notifications dropped due to full buffers across all subscribers.
	DroppedNotifications() uint64

	// Close closes the event pool
	Close() error
}

// eventPool implements the Pool interface
type eventPool struct {
	cfg    Config
	host   host.Host
	dag    dag.DAG
	beacon beacon.Beacon

	mu        sync.RWMutex
	running   bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup // tracks the run goroutine
	done      chan struct{}  // closed once Close is called
	closeOnce sync.Once
	eventCh   chan *dag.Event
	errCh     chan error // buffered channel for async errors
	beaconCh  <-chan *beacon.Round

	// Subscribers for event notifications
	subscribers []*subscription

	// Track recent events for sub-event relationships
	recentEvents []string

	// Track last beacon round used for anchoring
	lastBeaconRound uint64

	// Per-pool RNG (math/rand.Rand is not goroutine-safe — protect with rngMu)
	rngMu sync.Mutex
	rng   *rand.Rand

	// Atomic counter of subscriber notifications dropped due to full buffers.
	droppedNotifications uint64

	logger *slog.Logger
}

// subscription is the concrete Subscription implementation.
type subscription struct {
	ch   chan *dag.Event
	pool *eventPool
	once sync.Once
}

func (s *subscription) Events() <-chan *dag.Event { return s.ch }

func (s *subscription) Unsubscribe() {
	s.once.Do(func() { s.pool.removeSubscriber(s) })
}

const (
	// Maximum number of recent events to track
	maxRecentEvents = 100
	// Maximum number of parents for a sub-event
	maxSubEventParents = 3
	// Per-subscriber buffer size
	subscriberBufferSize = 100
)

// ErrInvalidConfig indicates the pool configuration is invalid
var ErrInvalidConfig = fmt.Errorf("invalid pool configuration")

// ErrPoolClosed indicates the pool has been closed
var ErrPoolClosed = fmt.Errorf("pool is closed")

// NewPool creates a new event pool
func NewPool(ctx context.Context, h host.Host, d dag.DAG, b beacon.Beacon, cfg Config) (Pool, error) {
	if h == nil {
		return nil, fmt.Errorf("%w: host cannot be nil", ErrInvalidConfig)
	}
	if d == nil {
		return nil, fmt.Errorf("%w: DAG cannot be nil", ErrInvalidConfig)
	}
	if b == nil {
		return nil, fmt.Errorf("%w: beacon cannot be nil", ErrInvalidConfig)
	}
	if cfg.EventRate <= 0 {
		return nil, fmt.Errorf("%w: EventRate must be positive", ErrInvalidConfig)
	}
	if cfg.AnchorInterval <= 0 {
		return nil, fmt.Errorf("%w: AnchorInterval must be positive", ErrInvalidConfig)
	}
	if cfg.SubEventComplex < 0 || cfg.SubEventComplex > 1 {
		return nil, fmt.Errorf("%w: SubEventComplex must be between 0 and 1", ErrInvalidConfig)
	}
	if cfg.VerifyInterval <= 0 {
		return nil, fmt.Errorf("%w: VerifyInterval must be positive", ErrInvalidConfig)
	}

	// Subscribe to beacon updates
	beaconCh, err := b.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to beacon: %w", err)
	}

	seed := cfg.RNGSeed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	logger := cfg.Logger
	if logger == nil {
		opts := &slog.HandlerOptions{}
		if cfg.Verbose {
			opts.Level = slog.LevelDebug
		}
		logger = slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	logger = logger.With("component", "pool")

	p := &eventPool{
		cfg:          cfg,
		host:         h,
		dag:          d,
		beacon:       b,
		eventCh:      make(chan *dag.Event, 100),
		errCh:        make(chan error, 100),
		beaconCh:     beaconCh,
		done:         make(chan struct{}),
		recentEvents: make([]string, 0, maxRecentEvents),
		rng:          rand.New(rand.NewSource(seed)),
		logger:       logger,
	}

	// Start processing events
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.running = true

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.run(ctx)
	}()

	return p, nil
}

// Errors returns a channel of asynchronous errors from the pool
func (p *eventPool) Errors() <-chan error {
	return p.errCh
}

// DroppedNotifications returns the cumulative dropped-notification count.
func (p *eventPool) DroppedNotifications() uint64 {
	return atomic.LoadUint64(&p.droppedNotifications)
}

// Subscribe returns a Subscription handle.
func (p *eventPool) Subscribe() (Subscription, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil, ErrPoolClosed
	}

	s := &subscription{
		ch:   make(chan *dag.Event, subscriberBufferSize),
		pool: p,
	}
	p.subscribers = append(p.subscribers, s)
	return s, nil
}

// removeSubscriber detaches a subscription and closes its channel.
// Idempotent at the eventPool level: if the subscription is no longer
// in the slice (e.g. already closed by Close), this is a no-op.
func (p *eventPool) removeSubscriber(s *subscription) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, existing := range p.subscribers {
		if existing == s {
			p.subscribers = append(p.subscribers[:i], p.subscribers[i+1:]...)
			close(s.ch)
			return
		}
	}
}

// notifySubscribers sends an event to all subscribers
func (p *eventPool) notifySubscribers(event *dag.Event) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, s := range p.subscribers {
		select {
		case s.ch <- event:
		default:
			// Subscriber channel full — drop and count
			atomic.AddUint64(&p.droppedNotifications, 1)
		}
	}
}

// sendError sends an error to the error channel if there is room
func (p *eventPool) sendError(err error) {
	select {
	case p.errCh <- err:
	default:
		// Channel full — log and drop.
		p.logger.Warn("error channel full, dropping error", "err", err)
	}
}

// computeEventCID computes and sets the content-addressed ID for an event.
// Returns an error if CID computation fails.
func computeEventCID(event *dag.Event) error {
	cid, err := dag.ComputeCID(event)
	if err != nil {
		return fmt.Errorf("failed to compute CID: %w", err)
	}
	event.ID = cid
	return nil
}

// rngFloat64 returns a random float64 from the per-pool RNG.
func (p *eventPool) rngFloat64() float64 {
	p.rngMu.Lock()
	defer p.rngMu.Unlock()
	return p.rng.Float64()
}

// rngIntn returns a random int in [0,n) from the per-pool RNG.
func (p *eventPool) rngIntn(n int) int {
	p.rngMu.Lock()
	defer p.rngMu.Unlock()
	return p.rng.Intn(n)
}

// shouldCreateSubEvent determines if we should create a sub-event
func (p *eventPool) shouldCreateSubEvent() bool {
	return p.rngFloat64() < p.cfg.SubEventComplex
}

// selectRandomParents selects random parent events from recent events
func (p *eventPool) selectRandomParents(count int) []string {
	p.mu.RLock()
	recent := p.recentEvents
	if len(recent) == 0 {
		p.mu.RUnlock()
		return nil
	}
	// Snapshot to avoid holding the lock across rng calls (which take their own lock).
	snapshot := make([]string, len(recent))
	copy(snapshot, recent)
	p.mu.RUnlock()

	parents := make([]string, 0, count)
	seen := make(map[string]bool)

	for i := 0; i < count && i < len(snapshot); i++ {
		// Try up to 5 times to find an unused parent
		for j := 0; j < 5; j++ {
			idx := p.rngIntn(len(snapshot))
			id := snapshot[idx]
			if !seen[id] {
				parents = append(parents, id)
				seen[id] = true
				break
			}
		}
	}

	return parents
}

// addRecentEvent adds an event ID to the recent events list
func (p *eventPool) addRecentEvent(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.recentEvents = append(p.recentEvents, id)
	if len(p.recentEvents) > maxRecentEvents {
		p.recentEvents = p.recentEvents[1:]
	}
}

// ensureBeaconRound gets a beacon round for anchoring, trying channel first then direct fetch
func (p *eventPool) ensureBeaconRound(ctx context.Context) (*beacon.Round, error) {
	// Try to get from channel first (most recent)
	select {
	case round := <-p.beaconCh:
		if err := p.validateBeaconRound(round); err != nil {
			return nil, fmt.Errorf("invalid beacon from channel: %w", err)
		}
		return round, nil
	default:
		// Channel is empty, fetch directly from beacon
		round, err := p.beacon.GetLatestRound(ctx)
		if err != nil {
			return nil, err
		}
		if err := p.validateBeaconRound(round); err != nil {
			return nil, fmt.Errorf("invalid beacon from API: %w", err)
		}
		return round, nil
	}
}

// validateBeaconRound performs basic validation on a beacon round
func (p *eventPool) validateBeaconRound(round *beacon.Round) error {
	if round == nil {
		return fmt.Errorf("beacon round is nil")
	}
	if round.Number == 0 {
		return fmt.Errorf("invalid round number: %d", round.Number)
	}
	if len(round.Randomness) == 0 {
		return fmt.Errorf("beacon randomness is empty")
	}
	if len(round.Signature) == 0 {
		return fmt.Errorf("beacon signature is empty")
	}

	// Check if beacon is reasonably recent (not older than 1 hour)
	maxAge := time.Hour
	age := time.Since(round.Timestamp)
	if age > maxAge {
		return fmt.Errorf("beacon round %d is too old: %v", round.Number, age)
	}

	return nil
}

func (p *eventPool) AddEvent(ctx context.Context, data []byte, parents []string) (string, error) {
	// Fast-path closed check (race-free because p.done is closed exactly once
	// and we never reopen it).
	select {
	case <-p.done:
		return "", ErrPoolClosed
	default:
	}

	event := &dag.Event{
		Type:    dag.MainEvent,
		Data:    data,
		Parents: parents,
	}

	// Compute content-addressed ID
	if err := computeEventCID(event); err != nil {
		return "", err
	}

	// p.eventCh is intentionally never closed — closing a channel with multiple
	// senders is unsafe. p.done is the shutdown signal instead.
	select {
	case p.eventCh <- event:
		return event.ID, nil
	case <-p.done:
		return "", ErrPoolClosed
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *eventPool) Close() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.running = false
		close(p.done)
		if p.cancel != nil {
			p.cancel()
		}
		p.mu.Unlock()

		// Wait for the run goroutine to exit before touching state it owns.
		p.wg.Wait()

		// Close errCh now that the only sender (run) has exited.
		// eventCh is intentionally NOT closed — multiple senders.
		close(p.errCh)

		// Close any remaining subscriber channels (those not Unsubscribed).
		p.mu.Lock()
		for _, s := range p.subscribers {
			close(s.ch)
		}
		p.subscribers = nil
		p.mu.Unlock()
	})
	return nil
}

func (p *eventPool) run(ctx context.Context) {
	eventTicker := time.NewTicker(time.Duration(p.cfg.EventRate) * time.Millisecond)
	verifyTicker := time.NewTicker(time.Duration(p.cfg.VerifyInterval) * time.Duration(p.cfg.EventRate) * time.Millisecond)
	defer eventTicker.Stop()
	defer verifyTicker.Stop()

	var eventCount int

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-p.eventCh:
			// Add the event to the DAG
			if err := p.dag.AddEvent(ctx, event); err != nil {
				p.sendError(fmt.Errorf("failed to add event %s: %w", event.ID, err))
				continue
			}

			// Notify subscribers of the new event
			p.notifySubscribers(event)

			p.logger.Debug("created main event", "id", event.ID)

			// Track the event
			p.addRecentEvent(event.ID)

			// Maybe create sub-events
			if p.shouldCreateSubEvent() {
				numSubEvents := p.rngIntn(3) + 1 // 1-3 sub-events
				p.logger.Debug("creating sub-events", "count", numSubEvents, "parent", event.ID)

				for i := 0; i < numSubEvents; i++ {
					subEvent := &dag.Event{
						Type:     dag.SubEvent,
						ParentID: event.ID,
						Data:     []byte(fmt.Sprintf("sub-event-%d", i)),
					}

					// Maybe connect to other sub-events
					if p.rngFloat64() < p.cfg.SubEventComplex {
						parentCount := p.rngIntn(maxSubEventParents) + 1
						subEvent.Parents = p.selectRandomParents(parentCount)
					}

					// Compute content-addressed ID (after parents are set)
					if err := computeEventCID(subEvent); err != nil {
						p.sendError(fmt.Errorf("failed to compute sub-event CID: %w", err))
						continue
					}

					if err := p.dag.AddEvent(ctx, subEvent); err != nil {
						p.sendError(fmt.Errorf("failed to add sub-event %s: %w", subEvent.ID, err))
						continue
					}

					// Notify subscribers of the new sub-event
					p.notifySubscribers(subEvent)

					p.logger.Debug("created sub-event",
						"id", subEvent.ID,
						"parent", subEvent.ParentID,
						"connects_to", subEvent.Parents)
					p.addRecentEvent(subEvent.ID)
				}
			}

			eventCount++
			if eventCount >= p.cfg.AnchorInterval {
				// ALWAYS anchor - this ensures deterministic spine intervals
				round, err := p.ensureBeaconRound(ctx)
				if err != nil {
					p.sendError(fmt.Errorf("failed to get beacon for anchoring: %w", err))
					// Continue without anchoring this time, but don't reset counter
					// This ensures we'll try again on the next event
				} else {
					// Validate monotonic progression - only anchor if beacon is newer
					if round.Number > p.lastBeaconRound {
						event.Beacon = &dag.BeaconAnchor{
							Round:      round.Number,
							Randomness: round.Randomness,
						}
						p.lastBeaconRound = round.Number
						eventCount = 0
						p.logger.Debug("anchored event", "event_id", event.ID, "drand_round", round.Number)
					} else {
						p.sendError(fmt.Errorf("stale beacon round %d (last: %d)", round.Number, p.lastBeaconRound))
						// Don't reset counter - we'll try again next time
					}
				}
			}

		case <-verifyTicker.C:
			if err := p.dag.Verify(ctx); err != nil {
				p.sendError(fmt.Errorf("DAG verification failed: %w", err))
			}
		}
	}
}
