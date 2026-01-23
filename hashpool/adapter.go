package hashpool

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/systemshift/dag-time/dag"
	"github.com/systemshift/dag-time/pool"
	"github.com/systemshift/hashpool/pkg/commitment"
	hashpoolnode "github.com/systemshift/hashpool/pkg/node"
)

// CommitmentEvent is the data structure stored in dag-time events for hashpool commitments
type CommitmentEvent struct {
	HashpoolRound uint64   `json:"hashpool_round"`
	MerkleRoot    string   `json:"merkle_root"`
	HashCount     int      `json:"hash_count"`
	NodeID        string   `json:"node_id"`
	Timestamp     string   `json:"timestamp"`
	Hashes        []string `json:"hashes,omitempty"`
}

// Adapter bridges hashpool commitments to dag-time events
type Adapter struct {
	cfg         Config
	hashpool    *hashpoolnode.Node
	pool        pool.Pool
	dag         dag.DAG
	lastEventID string

	mu      sync.RWMutex
	running bool
	cancel  context.CancelFunc
}

// NewAdapter creates a new hashpool-to-dagtime adapter
func NewAdapter(ctx context.Context, cfg Config, p pool.Pool, d dag.DAG) (*Adapter, error) {
	if p == nil {
		return nil, fmt.Errorf("pool cannot be nil")
	}
	if d == nil {
		return nil, fmt.Errorf("dag cannot be nil")
	}

	// Create hashpool node configuration
	hashpoolCfg := hashpoolnode.Config{
		ListenPort:     cfg.Hashpool.ListenPort,
		BootstrapPeers: cfg.Hashpool.BootstrapPeers,
		Difficulty:     cfg.Hashpool.Difficulty,
		RoundInterval:  cfg.Hashpool.RoundInterval,
		Verbose:        cfg.Verbose,
	}

	// Create hashpool node
	hpNode, err := hashpoolnode.New(ctx, hashpoolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create hashpool node: %w", err)
	}

	a := &Adapter{
		cfg:      cfg,
		hashpool: hpNode,
		pool:     p,
		dag:      d,
	}

	// Set up callback for commitments
	hpNode.SetOnCommitment(a.handleCommitment)

	return a, nil
}

// Start begins processing hashpool commitments
func (a *Adapter) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("adapter already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.running = true
	a.mu.Unlock()

	// Start hashpool node
	if err := a.hashpool.Start(ctx); err != nil {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		return fmt.Errorf("failed to start hashpool: %w", err)
	}

	if a.cfg.Verbose {
		log.Printf("Hashpool adapter started")
	}

	return nil
}

// Stop stops the adapter and hashpool node
func (a *Adapter) Stop() error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}
	a.cancel()
	a.running = false
	a.mu.Unlock()

	if a.cfg.Verbose {
		log.Printf("Hashpool adapter stopping")
	}

	return a.hashpool.Stop()
}

// handleCommitment processes a hashpool commitment and creates a dag-time event
func (a *Adapter) handleCommitment(c *commitment.Commitment) {
	a.mu.Lock()
	lastEventID := a.lastEventID
	a.mu.Unlock()

	// Build event data
	eventData := CommitmentEvent{
		HashpoolRound: c.Round,
		MerkleRoot:    hex.EncodeToString(c.Root[:]),
		HashCount:     len(c.Hashes),
		NodeID:        c.NodeID,
		Timestamp:     c.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
	}

	// Optionally include all hashes
	if a.cfg.IncludeFullHashes {
		eventData.Hashes = make([]string, len(c.Hashes))
		for i, h := range c.Hashes {
			eventData.Hashes[i] = hex.EncodeToString(h[:])
		}
	}

	// Serialize event data
	data, err := json.Marshal(eventData)
	if err != nil {
		log.Printf("Failed to marshal commitment event: %v", err)
		return
	}

	// Determine parents
	var parents []string
	if a.cfg.ChainCommitments && lastEventID != "" {
		parents = []string{lastEventID}
	}

	// Add event to dag-time
	ctx := context.Background()
	eventID, err := a.pool.AddEvent(ctx, data, parents)
	if err != nil {
		log.Printf("Failed to add commitment event to DAG: %v", err)
		return
	}

	// Update last event ID for chaining
	a.mu.Lock()
	a.lastEventID = eventID
	a.mu.Unlock()

	if a.cfg.Verbose {
		log.Printf("Created dag-time event %s for hashpool round %d (%d hashes)",
			eventID, c.Round, len(c.Hashes))
	}
}

// Submit submits a hash to the hashpool
func (a *Adapter) Submit(hash [32]byte, nonce uint64) error {
	return a.hashpool.Submit(hash, nonce)
}

// Difficulty returns the current PoW difficulty
func (a *Adapter) Difficulty() uint8 {
	return a.hashpool.Difficulty()
}

// HashpoolNode returns the underlying hashpool node
func (a *Adapter) HashpoolNode() *hashpoolnode.Node {
	return a.hashpool
}

// LastEventID returns the ID of the last created event
func (a *Adapter) LastEventID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastEventID
}
