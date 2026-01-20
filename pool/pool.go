// Package pool manages event synchronization between nodes
package pool

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/systemshift/dag-time/beacon"
	"github.com/systemshift/dag-time/dag"
)

// Create a local random number generator
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

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

	// Verbose enables detailed logging of event relationships
	Verbose bool
}

// Pool manages a pool of events and handles event synchronization
type Pool interface {
	// AddEvent adds a new event to the pool
	AddEvent(ctx context.Context, data []byte, parents []string) error

	// Close closes the event pool
	Close() error
}

// eventPool implements the Pool interface
type eventPool struct {
	cfg    Config
	host   host.Host
	dag    dag.DAG
	beacon beacon.Beacon

	mu       sync.RWMutex
	running  bool
	cancel   context.CancelFunc
	eventCh  chan *dag.Event
	beaconCh <-chan *beacon.Round

	// Track recent events for sub-event relationships
	recentEvents []string

	// Track last beacon round used for anchoring
	lastBeaconRound uint64
}

const (
	// Maximum number of recent events to track
	maxRecentEvents = 100
	// Maximum number of parents for a sub-event
	maxSubEventParents = 3
)

// ErrInvalidConfig indicates the pool configuration is invalid
var ErrInvalidConfig = fmt.Errorf("invalid pool configuration")

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

	p := &eventPool{
		cfg:          cfg,
		host:         h,
		dag:          d,
		beacon:       b,
		eventCh:      make(chan *dag.Event, 100),
		beaconCh:     beaconCh,
		recentEvents: make([]string, 0, maxRecentEvents),
	}

	// Start processing events
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.running = true

	go p.run(ctx)

	return p, nil
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

// shouldCreateSubEvent determines if we should create a sub-event
func (p *eventPool) shouldCreateSubEvent() bool {
	return rng.Float64() < p.cfg.SubEventComplex
}

// selectRandomParents selects random parent events from recent events
func (p *eventPool) selectRandomParents(count int) []string {
	if len(p.recentEvents) == 0 {
		return nil
	}

	// Randomly select up to count parents
	parents := make([]string, 0, count)
	seen := make(map[string]bool)

	for i := 0; i < count && i < len(p.recentEvents); i++ {
		// Try up to 5 times to find an unused parent
		for j := 0; j < 5; j++ {
			idx := rng.Intn(len(p.recentEvents))
			id := p.recentEvents[idx]
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

func (p *eventPool) AddEvent(ctx context.Context, data []byte, parents []string) error {
	if !p.running {
		return fmt.Errorf("pool is not running")
	}

	event := &dag.Event{
		Type:    dag.MainEvent,
		Data:    data,
		Parents: parents,
	}

	// Compute content-addressed ID
	if err := computeEventCID(event); err != nil {
		return err
	}

	select {
	case p.eventCh <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *eventPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.cancel()
	p.running = false
	close(p.eventCh)

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
				log.Printf("Failed to add event: %v", err)
				continue
			}

			if p.cfg.Verbose {
				log.Printf("Created main event %s", event.ID)
			}

			// Track the event
			p.addRecentEvent(event.ID)

			// Maybe create sub-events
			if p.shouldCreateSubEvent() {
				numSubEvents := rng.Intn(3) + 1 // 1-3 sub-events
				if p.cfg.Verbose {
					log.Printf("Creating %d sub-events for %s", numSubEvents, event.ID)
				}

				for i := 0; i < numSubEvents; i++ {
					subEvent := &dag.Event{
						Type:     dag.SubEvent,
						ParentID: event.ID,
						Data:     []byte(fmt.Sprintf("sub-event-%d", i)),
					}

					// Maybe connect to other sub-events
					if rng.Float64() < p.cfg.SubEventComplex {
						parentCount := rng.Intn(maxSubEventParents) + 1
						subEvent.Parents = p.selectRandomParents(parentCount)
					}

					// Compute content-addressed ID (after parents are set)
					if err := computeEventCID(subEvent); err != nil {
						log.Printf("Failed to compute sub-event CID: %v", err)
						continue
					}

					if p.cfg.Verbose && len(subEvent.Parents) > 0 {
						log.Printf("  Sub-event %s connects to: %v", subEvent.ID, subEvent.Parents)
					}

					if err := p.dag.AddEvent(ctx, subEvent); err != nil {
						log.Printf("Failed to add sub-event: %v", err)
						continue
					}
					if p.cfg.Verbose {
						log.Printf("  Created sub-event %s", subEvent.ID)
					}
					p.addRecentEvent(subEvent.ID)
				}
			}

			eventCount++
			if eventCount >= p.cfg.AnchorInterval {
				// ALWAYS anchor - this ensures deterministic spine intervals
				round, err := p.ensureBeaconRound(ctx)
				if err != nil {
					log.Printf("Failed to get beacon for anchoring: %v", err)
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
						if p.cfg.Verbose {
							log.Printf("Anchored event %s to drand round %d", event.ID, round.Number)
						}
					} else {
						log.Printf("Skipping stale beacon round %d (last: %d)", round.Number, p.lastBeaconRound)
						// Don't reset counter - we'll try again next time
					}
				}
			}

		case <-verifyTicker.C:
			if err := p.dag.Verify(ctx); err != nil {
				log.Printf("DAG verification failed: %v", err)
			}
		}
	}
}
