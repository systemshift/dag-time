// Package pool manages event synchronization between nodes
package pool

import (
	"context"
	"fmt"
	"sync"
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
}

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
		cfg:      cfg,
		host:     h,
		dag:      d,
		beacon:   b,
		eventCh:  make(chan *dag.Event, 100),
		beaconCh: beaconCh,
	}

	// Start processing events
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.running = true

	go p.run(ctx)

	return p, nil
}

func (p *eventPool) AddEvent(ctx context.Context, data []byte, parents []string) error {
	if !p.running {
		return fmt.Errorf("pool is not running")
	}

	event := &dag.Event{
		Data:    data,
		Parents: parents,
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
			if err := p.dag.AddEvent(ctx, event); err != nil {
				// TODO: Handle error (retry, log, etc)
				continue
			}

			eventCount++
			if eventCount >= p.cfg.AnchorInterval {
				// Anchor to latest beacon round
				select {
				case round := <-p.beaconCh:
					event.Beacon = &dag.BeaconAnchor{
						Round:      round.Number,
						Randomness: round.Randomness,
					}
					eventCount = 0
				default:
					// No beacon round available, continue
				}
			}

		case <-verifyTicker.C:
			if err := p.dag.Verify(ctx); err != nil {
				// TODO: Handle verification failure
			}
		}
	}
}
