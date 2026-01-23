package hashpool

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/systemshift/dag-time/dag"
	"github.com/systemshift/hashpool/pkg/pow"
)

// Integration tests for the hashpool adapter
// These tests verify the full flow from hash submission to dag-time event creation

func TestIntegrationHashSubmissionCreatesEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create real DAG
	memDAG := dag.NewMemoryDAG()

	// Create mock pool that stores events in the real DAG
	pool := &integrationPool{
		dag:     memDAG,
		eventCh: make(chan *dag.Event, 100),
	}

	// Create adapter with short round interval for testing
	cfg := Config{
		Hashpool: HashpoolConfig{
			ListenPort:    0, // Random port
			Difficulty:    8, // Low difficulty for fast tests
			RoundInterval: 500 * time.Millisecond,
		},
		IncludeFullHashes: true,
		ChainCommitments:  true,
		Verbose:           false,
	}

	adapter, err := NewAdapter(ctx, cfg, pool, memDAG)
	require.NoError(t, err)

	// Start the adapter
	err = adapter.Start(ctx)
	require.NoError(t, err)
	defer adapter.Stop()

	// Create and solve a hash with PoW
	data := []byte("test data for integration")
	hash := sha256.Sum256(data)
	challenge := pow.Solve(hash, cfg.Hashpool.Difficulty)

	// Submit the hash
	err = adapter.Submit(challenge.Hash, challenge.Nonce)
	require.NoError(t, err)

	// Wait for commitment round (2x round interval to be safe)
	time.Sleep(cfg.Hashpool.RoundInterval * 3)

	// Verify event was created
	events, err := memDAG.GetMainEvents(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, events, "expected at least one event in DAG")

	// Find the commitment event
	var commitmentEvent *dag.Event
	for _, e := range events {
		var ce CommitmentEvent
		if err := json.Unmarshal(e.Data, &ce); err == nil {
			if ce.HashCount > 0 {
				commitmentEvent = e
				break
			}
		}
	}
	require.NotNil(t, commitmentEvent, "expected to find commitment event")

	// Verify event data
	var eventData CommitmentEvent
	err = json.Unmarshal(commitmentEvent.Data, &eventData)
	require.NoError(t, err)

	assert.Greater(t, eventData.HashpoolRound, uint64(0))
	assert.NotEmpty(t, eventData.MerkleRoot)
	assert.Equal(t, 1, eventData.HashCount)
	assert.NotEmpty(t, eventData.NodeID)
	assert.NotEmpty(t, eventData.Timestamp)
	assert.Len(t, eventData.Hashes, 1, "IncludeFullHashes is true")
}

func TestIntegrationMultipleHashesInSameRound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	memDAG := dag.NewMemoryDAG()
	pool := &integrationPool{
		dag:     memDAG,
		eventCh: make(chan *dag.Event, 100),
	}

	cfg := Config{
		Hashpool: HashpoolConfig{
			ListenPort:    0,
			Difficulty:    8,
			RoundInterval: 1 * time.Second, // Longer to batch hashes
		},
		IncludeFullHashes: true,
		ChainCommitments:  true,
	}

	adapter, err := NewAdapter(ctx, cfg, pool, memDAG)
	require.NoError(t, err)

	err = adapter.Start(ctx)
	require.NoError(t, err)
	defer adapter.Stop()

	// Submit multiple hashes quickly
	numHashes := 3
	for i := 0; i < numHashes; i++ {
		data := []byte{byte(i), byte(i + 1), byte(i + 2)}
		hash := sha256.Sum256(data)
		challenge := pow.Solve(hash, cfg.Hashpool.Difficulty)
		err = adapter.Submit(challenge.Hash, challenge.Nonce)
		require.NoError(t, err)
	}

	// Wait for commitment
	time.Sleep(cfg.Hashpool.RoundInterval * 2)

	// Verify event contains all hashes
	events, err := memDAG.GetMainEvents(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, events)

	// Find commitment with multiple hashes
	var found bool
	for _, e := range events {
		var ce CommitmentEvent
		if err := json.Unmarshal(e.Data, &ce); err == nil {
			if ce.HashCount == numHashes {
				found = true
				assert.Len(t, ce.Hashes, numHashes)
				break
			}
		}
	}
	assert.True(t, found, "expected to find commitment with %d hashes", numHashes)
}

func TestIntegrationCommitmentChaining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	memDAG := dag.NewMemoryDAG()
	pool := &integrationPool{
		dag:     memDAG,
		eventCh: make(chan *dag.Event, 100),
	}

	cfg := Config{
		Hashpool: HashpoolConfig{
			ListenPort:    0,
			Difficulty:    8,
			RoundInterval: 500 * time.Millisecond,
		},
		IncludeFullHashes: false,
		ChainCommitments:  true,
	}

	adapter, err := NewAdapter(ctx, cfg, pool, memDAG)
	require.NoError(t, err)

	err = adapter.Start(ctx)
	require.NoError(t, err)
	defer adapter.Stop()

	// Submit hash for first round
	hash1 := sha256.Sum256([]byte("first"))
	challenge1 := pow.Solve(hash1, cfg.Hashpool.Difficulty)
	err = adapter.Submit(challenge1.Hash, challenge1.Nonce)
	require.NoError(t, err)

	// Wait for first commitment
	time.Sleep(cfg.Hashpool.RoundInterval * 2)

	firstEventID := adapter.LastEventID()
	require.NotEmpty(t, firstEventID, "expected first event ID")

	// Submit hash for second round
	hash2 := sha256.Sum256([]byte("second"))
	challenge2 := pow.Solve(hash2, cfg.Hashpool.Difficulty)
	err = adapter.Submit(challenge2.Hash, challenge2.Nonce)
	require.NoError(t, err)

	// Wait for second commitment
	time.Sleep(cfg.Hashpool.RoundInterval * 2)

	secondEventID := adapter.LastEventID()
	require.NotEmpty(t, secondEventID)
	require.NotEqual(t, firstEventID, secondEventID)

	// Verify second event has first as parent
	events, err := memDAG.GetMainEvents(ctx)
	require.NoError(t, err)

	var secondEvent *dag.Event
	for _, e := range events {
		if e.ID == secondEventID {
			secondEvent = e
			break
		}
	}
	require.NotNil(t, secondEvent)
	require.Contains(t, secondEvent.Parents, firstEventID,
		"second commitment should have first as parent")
}

func TestIntegrationInvalidPoWRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	memDAG := dag.NewMemoryDAG()
	pool := &integrationPool{
		dag:     memDAG,
		eventCh: make(chan *dag.Event, 100),
	}

	cfg := Config{
		Hashpool: HashpoolConfig{
			ListenPort:    0,
			Difficulty:    16, // Higher difficulty
			RoundInterval: 500 * time.Millisecond,
		},
		ChainCommitments: true,
	}

	adapter, err := NewAdapter(ctx, cfg, pool, memDAG)
	require.NoError(t, err)

	err = adapter.Start(ctx)
	require.NoError(t, err)
	defer adapter.Stop()

	// Submit hash with invalid nonce (no PoW)
	hash := sha256.Sum256([]byte("test"))
	err = adapter.Submit(hash, 0) // Nonce 0 is unlikely to satisfy PoW
	// The submission might not return error immediately (mempool validates async)
	// But the hash should not appear in any commitment

	// Wait for potential commitment
	time.Sleep(cfg.Hashpool.RoundInterval * 2)

	// Should have no events (invalid PoW rejected)
	events, err := memDAG.GetMainEvents(ctx)
	require.NoError(t, err)

	// Check no commitment contains our invalid hash
	for _, e := range events {
		var ce CommitmentEvent
		if err := json.Unmarshal(e.Data, &ce); err == nil {
			// If there are hashes, none should be our invalid submission
			// (unless by coincidence nonce 0 satisfies the PoW, which is very unlikely)
			assert.Equal(t, 0, ce.HashCount, "no valid hashes should be committed")
		}
	}
}

func TestIntegrationDifficulty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	memDAG := dag.NewMemoryDAG()
	pool := &integrationPool{
		dag:     memDAG,
		eventCh: make(chan *dag.Event, 100),
	}

	cfg := Config{
		Hashpool: HashpoolConfig{
			ListenPort:    0,
			Difficulty:    12,
			RoundInterval: 1 * time.Second,
		},
	}

	adapter, err := NewAdapter(ctx, cfg, pool, memDAG)
	require.NoError(t, err)

	// Verify difficulty is accessible
	assert.Equal(t, uint8(12), adapter.Difficulty())
}

// integrationPool wraps a real DAG for integration testing
type integrationPool struct {
	dag       dag.DAG
	eventCh   chan *dag.Event
	eventID   int
	lastEvent *dag.Event
}

func (p *integrationPool) AddEvent(ctx context.Context, data []byte, parents []string) (string, error) {
	p.eventID++
	id := computeTestCID(data, parents, p.eventID)

	event := &dag.Event{
		ID:      id,
		Type:    dag.MainEvent,
		Data:    data,
		Parents: parents,
	}

	if err := p.dag.AddEvent(ctx, event); err != nil {
		return "", err
	}

	p.lastEvent = event

	// Notify subscribers
	select {
	case p.eventCh <- event:
	default:
	}

	return id, nil
}

func (p *integrationPool) Subscribe() (<-chan *dag.Event, error) {
	return p.eventCh, nil
}

func (p *integrationPool) Errors() <-chan error {
	return make(chan error)
}

func (p *integrationPool) Close() error {
	close(p.eventCh)
	return nil
}

// computeTestCID creates a deterministic test CID
func computeTestCID(data []byte, parents []string, seq int) string {
	h := sha256.New()
	h.Write(data)
	for _, p := range parents {
		h.Write([]byte(p))
	}
	h.Write([]byte{byte(seq)})
	sum := h.Sum(nil)
	return "test-" + string(encodeHex(sum[:8]))
}

func encodeHex(b []byte) []byte {
	const hex = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hex[v>>4]
		result[i*2+1] = hex[v&0x0f]
	}
	return result
}
