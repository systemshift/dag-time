package hashpool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/systemshift/dag-time/dag"
	"github.com/systemshift/hashpool/pkg/commitment"
)

// Mock pool implementation for testing
type mockPool struct {
	events  []mockEvent
	closed  bool
	eventCh chan *dag.Event
}

type mockEvent struct {
	data    []byte
	parents []string
	id      string
}

func newMockPool() *mockPool {
	return &mockPool{
		events:  make([]mockEvent, 0),
		eventCh: make(chan *dag.Event, 100),
	}
}

func (m *mockPool) AddEvent(ctx context.Context, data []byte, parents []string) (string, error) {
	id := "mock-event-" + time.Now().Format("150405.000")
	m.events = append(m.events, mockEvent{
		data:    data,
		parents: parents,
		id:      id,
	})
	return id, nil
}

func (m *mockPool) Subscribe() (<-chan *dag.Event, error) {
	return m.eventCh, nil
}

func (m *mockPool) Errors() <-chan error {
	return make(chan error)
}

func (m *mockPool) Close() error {
	m.closed = true
	close(m.eventCh)
	return nil
}

// Mock DAG implementation for testing
type mockDAG struct {
	events map[string]*dag.Event
}

func newMockDAG() *mockDAG {
	return &mockDAG{
		events: make(map[string]*dag.Event),
	}
}

func (m *mockDAG) AddEvent(ctx context.Context, event *dag.Event) error {
	m.events[event.ID] = event
	return nil
}

func (m *mockDAG) GetEvent(ctx context.Context, id string) (*dag.Event, error) {
	event, ok := m.events[id]
	if !ok {
		return nil, dag.ErrNotFound
	}
	return event, nil
}

func (m *mockDAG) GetParents(ctx context.Context, id string) ([]*dag.Event, error) {
	return nil, nil
}

func (m *mockDAG) GetChildren(ctx context.Context, id string) ([]*dag.Event, error) {
	return nil, nil
}

func (m *mockDAG) GetSubEvents(ctx context.Context, id string) ([]*dag.Event, error) {
	return nil, nil
}

func (m *mockDAG) GetMainEvents(ctx context.Context) ([]*dag.Event, error) {
	events := make([]*dag.Event, 0, len(m.events))
	for _, e := range m.events {
		events = append(events, e)
	}
	return events, nil
}

func (m *mockDAG) Verify(ctx context.Context) error {
	return nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 0, cfg.Hashpool.ListenPort)
	assert.Equal(t, uint8(16), cfg.Hashpool.Difficulty)
	assert.Equal(t, 10*time.Second, cfg.Hashpool.RoundInterval)
	assert.False(t, cfg.IncludeFullHashes)
	assert.True(t, cfg.ChainCommitments)
	assert.False(t, cfg.Verbose)
}

func TestCommitmentEventJSON(t *testing.T) {
	event := CommitmentEvent{
		HashpoolRound: 123,
		MerkleRoot:    "abcd1234",
		HashCount:     10,
		NodeID:        "node-1",
		Timestamp:     "2024-01-15T10:30:00.000Z",
		Hashes:        []string{"hash1", "hash2"},
	}

	// Test serialization
	data, err := json.Marshal(event)
	require.NoError(t, err)

	// Test deserialization
	var decoded CommitmentEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, event.HashpoolRound, decoded.HashpoolRound)
	assert.Equal(t, event.MerkleRoot, decoded.MerkleRoot)
	assert.Equal(t, event.HashCount, decoded.HashCount)
	assert.Equal(t, event.NodeID, decoded.NodeID)
	assert.Equal(t, event.Timestamp, decoded.Timestamp)
	assert.Equal(t, event.Hashes, decoded.Hashes)
}

func TestCommitmentEventJSONWithoutHashes(t *testing.T) {
	event := CommitmentEvent{
		HashpoolRound: 456,
		MerkleRoot:    "efgh5678",
		HashCount:     5,
		NodeID:        "node-2",
		Timestamp:     "2024-01-15T11:00:00.000Z",
	}

	// Test serialization
	data, err := json.Marshal(event)
	require.NoError(t, err)

	// Verify hashes field is omitted when empty
	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)
	_, hasHashes := raw["hashes"]
	assert.False(t, hasHashes, "hashes field should be omitted when nil")
}

func TestAdapterNewNilPool(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	_, err := NewAdapter(ctx, cfg, nil, newMockDAG())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pool cannot be nil")
}

func TestAdapterNewNilDAG(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()

	_, err := NewAdapter(ctx, cfg, newMockPool(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dag cannot be nil")
}

func TestHandleCommitmentCreatesEvent(t *testing.T) {
	pool := newMockPool()

	// Create a minimal adapter to test handleCommitment directly
	a := &Adapter{
		cfg: Config{
			IncludeFullHashes: false,
			ChainCommitments:  true,
		},
		pool: pool,
		dag:  newMockDAG(),
	}

	// Create a test commitment
	hashes := [][32]byte{
		sha256Hash("hash1"),
		sha256Hash("hash2"),
	}
	c, err := commitment.New(1, time.Now(), hashes, "test-node")
	require.NoError(t, err)

	// Handle the commitment
	a.handleCommitment(c)

	// Verify an event was created
	require.Len(t, pool.events, 1)

	// Verify event data
	var eventData CommitmentEvent
	err = json.Unmarshal(pool.events[0].data, &eventData)
	require.NoError(t, err)

	assert.Equal(t, uint64(1), eventData.HashpoolRound)
	assert.Equal(t, 2, eventData.HashCount)
	assert.Equal(t, "test-node", eventData.NodeID)
	assert.Nil(t, eventData.Hashes) // IncludeFullHashes is false
}

func TestHandleCommitmentWithFullHashes(t *testing.T) {
	pool := newMockPool()

	a := &Adapter{
		cfg: Config{
			IncludeFullHashes: true,
			ChainCommitments:  false,
		},
		pool: pool,
		dag:  newMockDAG(),
	}

	hashes := [][32]byte{
		sha256Hash("hash1"),
		sha256Hash("hash2"),
	}
	c, err := commitment.New(1, time.Now(), hashes, "test-node")
	require.NoError(t, err)

	a.handleCommitment(c)

	require.Len(t, pool.events, 1)

	var eventData CommitmentEvent
	err = json.Unmarshal(pool.events[0].data, &eventData)
	require.NoError(t, err)

	assert.Len(t, eventData.Hashes, 2)
}

func TestHandleCommitmentChaining(t *testing.T) {
	pool := newMockPool()

	a := &Adapter{
		cfg: Config{
			IncludeFullHashes: false,
			ChainCommitments:  true,
		},
		pool: pool,
		dag:  newMockDAG(),
	}

	// First commitment
	hashes := [][32]byte{sha256Hash("hash1")}
	c1, err := commitment.New(1, time.Now(), hashes, "test-node")
	require.NoError(t, err)
	a.handleCommitment(c1)

	// Second commitment should have first as parent
	c2, err := commitment.New(2, time.Now(), hashes, "test-node")
	require.NoError(t, err)
	a.handleCommitment(c2)

	require.Len(t, pool.events, 2)

	// First event should have no parents
	assert.Empty(t, pool.events[0].parents)

	// Second event should have first as parent
	require.Len(t, pool.events[1].parents, 1)
	assert.Equal(t, pool.events[0].id, pool.events[1].parents[0])
}

func TestHandleCommitmentNoChainingNoParents(t *testing.T) {
	pool := newMockPool()

	a := &Adapter{
		cfg: Config{
			IncludeFullHashes: false,
			ChainCommitments:  false,
		},
		pool: pool,
		dag:  newMockDAG(),
	}

	hashes := [][32]byte{sha256Hash("hash1")}
	c1, err := commitment.New(1, time.Now(), hashes, "test-node")
	require.NoError(t, err)
	a.handleCommitment(c1)

	c2, err := commitment.New(2, time.Now(), hashes, "test-node")
	require.NoError(t, err)
	a.handleCommitment(c2)

	require.Len(t, pool.events, 2)

	// Neither event should have parents when chaining is disabled
	assert.Empty(t, pool.events[0].parents)
	assert.Empty(t, pool.events[1].parents)
}

func TestLastEventID(t *testing.T) {
	pool := newMockPool()

	a := &Adapter{
		cfg: Config{
			ChainCommitments: true,
		},
		pool: pool,
		dag:  newMockDAG(),
	}

	// Initially empty
	assert.Empty(t, a.LastEventID())

	// After handling a commitment
	hashes := [][32]byte{sha256Hash("hash1")}
	c, err := commitment.New(1, time.Now(), hashes, "test-node")
	require.NoError(t, err)
	a.handleCommitment(c)

	// Should have the event ID
	assert.NotEmpty(t, a.LastEventID())
	assert.Equal(t, pool.events[0].id, a.LastEventID())
}

// Helper function to create a test hash
func sha256Hash(data string) [32]byte {
	var hash [32]byte
	copy(hash[:], []byte(data))
	return hash
}
