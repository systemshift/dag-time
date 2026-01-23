// Package node provides the high-level interface for running a DAG-Time node
package node

import (
	"context"
	"fmt"
	"time"

	"github.com/systemshift/dag-time/beacon"
	"github.com/systemshift/dag-time/dag"
	"github.com/systemshift/dag-time/hashpool"
	"github.com/systemshift/dag-time/network"
	"github.com/systemshift/dag-time/pool"
)

// Config represents node configuration
type Config struct {
	// Network configuration
	Network network.Config

	// Beacon configuration
	BeaconURL         string
	BeaconInterval    time.Duration
	BeaconChainHash   []byte
	BeaconPublicKey   []byte
	BeaconGenesisTime int64

	// Pool configuration
	EventRate       int64
	AnchorInterval  int
	SubEventComplex float64
	VerifyInterval  int
	Verbose         bool

	// Hashpool configuration
	EnableHashpool bool
	HashpoolConfig *hashpool.Config
}

// Node represents a complete DAG-Time node
type Node struct {
	network         network.Node
	dag             dag.DAG
	beacon          beacon.Beacon
	pool            pool.Pool
	hashpoolAdapter *hashpool.Adapter
}

// ErrInvalidConfig indicates the node configuration is invalid
var ErrInvalidConfig = fmt.Errorf("invalid node configuration")

// ValidateConfig validates the node configuration
func ValidateConfig(cfg Config) error {
	if cfg.BeaconURL == "" {
		return fmt.Errorf("%w: BeaconURL cannot be empty", ErrInvalidConfig)
	}
	if cfg.BeaconInterval < time.Second {
		return fmt.Errorf("%w: BeaconInterval must be at least 1 second", ErrInvalidConfig)
	}
	if cfg.EventRate <= 0 {
		return fmt.Errorf("%w: EventRate must be positive", ErrInvalidConfig)
	}
	if cfg.AnchorInterval <= 0 {
		return fmt.Errorf("%w: AnchorInterval must be positive", ErrInvalidConfig)
	}
	if cfg.SubEventComplex < 0 || cfg.SubEventComplex > 1 {
		return fmt.Errorf("%w: SubEventComplex must be between 0 and 1", ErrInvalidConfig)
	}
	if cfg.VerifyInterval <= 0 {
		return fmt.Errorf("%w: VerifyInterval must be positive", ErrInvalidConfig)
	}
	return nil
}

// New creates a new DAG-Time node
func New(ctx context.Context, cfg Config) (*Node, error) {
	// Validate configuration
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	// Create network node
	n, err := network.NewNode(ctx, cfg.Network)
	if err != nil {
		return nil, fmt.Errorf("failed to create network node: %w", err)
	}

	// Create DAG
	d := dag.NewMemoryDAG()

	// Create beacon
	b, err := beacon.NewBeacon(beacon.Config{
		URL:         cfg.BeaconURL,
		Period:      cfg.BeaconInterval,
		ChainHash:   cfg.BeaconChainHash,
		PublicKey:   cfg.BeaconPublicKey,
		GenesisTime: cfg.BeaconGenesisTime,
	})
	if err != nil {
		n.Close()
		return nil, fmt.Errorf("failed to create beacon: %w", err)
	}

	// Start beacon
	if err := b.Start(ctx, cfg.BeaconInterval); err != nil {
		n.Close()
		return nil, fmt.Errorf("failed to start beacon: %w", err)
	}

	// Create pool
	p, err := pool.NewPool(ctx, n.Host(), d, b, pool.Config{
		EventRate:       cfg.EventRate,
		AnchorInterval:  cfg.AnchorInterval,
		SubEventComplex: cfg.SubEventComplex,
		VerifyInterval:  cfg.VerifyInterval,
		Verbose:         cfg.Verbose,
	})
	if err != nil {
		if err := b.Stop(); err != nil {
			n.Close()
			return nil, fmt.Errorf("failed to stop beacon: %w", err)
		}
		if err := n.Close(); err != nil {
			return nil, fmt.Errorf("failed to close network: %w", err)
		}
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	node := &Node{
		network: n,
		dag:     d,
		beacon:  b,
		pool:    p,
	}

	// Optionally create hashpool adapter
	if cfg.EnableHashpool {
		hpCfg := hashpool.DefaultConfig()
		if cfg.HashpoolConfig != nil {
			hpCfg = *cfg.HashpoolConfig
		}

		adapter, err := hashpool.NewAdapter(ctx, hpCfg, p, d)
		if err != nil {
			if closeErr := p.Close(); closeErr != nil {
				errs := []error{fmt.Errorf("failed to close pool: %w", closeErr)}
				errs = append(errs, fmt.Errorf("failed to create hashpool adapter: %w", err))
				return nil, fmt.Errorf("errors during cleanup: %v", errs)
			}
			if closeErr := b.Stop(); closeErr != nil {
				n.Close()
				return nil, fmt.Errorf("failed to stop beacon: %w", closeErr)
			}
			n.Close()
			return nil, fmt.Errorf("failed to create hashpool adapter: %w", err)
		}

		// Start the adapter
		if err := adapter.Start(ctx); err != nil {
			adapter.Stop()
			p.Close()
			b.Stop()
			n.Close()
			return nil, fmt.Errorf("failed to start hashpool adapter: %w", err)
		}

		node.hashpoolAdapter = adapter
	}

	return node, nil
}

// AddEvent adds a new event to the node's DAG and returns the event ID
func (n *Node) AddEvent(ctx context.Context, data []byte, parents []string) (string, error) {
	return n.pool.AddEvent(ctx, data, parents)
}

// GetEvent retrieves an event by ID from the node's DAG
func (n *Node) GetEvent(ctx context.Context, id string) (*dag.Event, error) {
	return n.dag.GetEvent(ctx, id)
}

// GetMainEvents returns all main events in the node's DAG
func (n *Node) GetMainEvents(ctx context.Context) ([]*dag.Event, error) {
	return n.dag.GetMainEvents(ctx)
}

// DAG returns the underlying DAG interface
func (n *Node) DAG() dag.DAG {
	return n.dag
}

// Subscribe returns a channel that receives events as they are added to the DAG
func (n *Node) Subscribe() (<-chan *dag.Event, error) {
	return n.pool.Subscribe()
}

// HashpoolAdapter returns the hashpool adapter if enabled, nil otherwise
func (n *Node) HashpoolAdapter() *hashpool.Adapter {
	return n.hashpoolAdapter
}

// SubmitHash submits a hash to the hashpool
// Returns an error if hashpool is not enabled
func (n *Node) SubmitHash(hash [32]byte, nonce uint64) error {
	if n.hashpoolAdapter == nil {
		return fmt.Errorf("hashpool is not enabled")
	}
	return n.hashpoolAdapter.Submit(hash, nonce)
}

// Close shuts down the node and all its components
func (n *Node) Close() error {
	var errs []error

	// Stop hashpool adapter first
	if n.hashpoolAdapter != nil {
		if err := n.hashpoolAdapter.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop hashpool adapter: %w", err))
		}
	}

	if n.pool != nil {
		if err := n.pool.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close pool: %w", err))
		}
	}

	if n.beacon != nil {
		if err := n.beacon.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop beacon: %w", err))
		}
	}

	if n.network != nil {
		if err := n.network.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close network: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing node: %v", errs)
	}

	return nil
}
