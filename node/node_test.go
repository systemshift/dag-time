package node

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"

	"github.com/systemshift/dag-time/beacon"
	"github.com/systemshift/dag-time/dag"
	"github.com/systemshift/dag-time/network"
	"github.com/systemshift/dag-time/pool"
)

// Mock implementations

type mockComponents struct {
	network *mockNetwork
	dag     *mockDAG
	beacon  *mockBeacon
	pool    *mockPool
}

func newMockComponents() *mockComponents {
	return &mockComponents{
		network: &mockNetwork{},
		dag:     &mockDAG{},
		beacon:  &mockBeacon{},
		pool:    &mockPool{},
	}
}

type mockNetwork struct {
	closed bool
}

func (m *mockNetwork) Host() host.Host                                             { return nil }
func (m *mockNetwork) Connect(ctx context.Context, addr multiaddr.Multiaddr) error { return nil }
func (m *mockNetwork) Peers() []peer.ID                                            { return nil }
func (m *mockNetwork) Close() error {
	m.closed = true
	return nil
}

type mockDAG struct {
	events map[string]*dag.Event
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
	return nil, nil
}

func (m *mockDAG) Verify(ctx context.Context) error {
	return nil
}

type mockBeacon struct {
	started bool
	stopped bool
}

func (m *mockBeacon) GetLatestRound(ctx context.Context) (*beacon.Round, error) {
	return &beacon.Round{Number: 1}, nil
}

func (m *mockBeacon) GetRound(ctx context.Context, round uint64) (*beacon.Round, error) {
	return &beacon.Round{Number: round}, nil
}

func (m *mockBeacon) Start(ctx context.Context, interval time.Duration) error {
	m.started = true
	return nil
}

func (m *mockBeacon) Stop() error {
	m.stopped = true
	return nil
}

func (m *mockBeacon) Subscribe() (<-chan *beacon.Round, error) {
	ch := make(chan *beacon.Round, 1)
	return ch, nil
}

type mockPool struct {
	closed bool
}

func (m *mockPool) AddEvent(ctx context.Context, data []byte, parents []string) (string, error) {
	if m.closed {
		return "", pool.ErrPoolClosed
	}
	return "mock-event-id", nil
}

func (m *mockPool) Subscribe() (<-chan *dag.Event, error) {
	if m.closed {
		return nil, pool.ErrPoolClosed
	}
	return make(chan *dag.Event), nil
}

func (m *mockPool) Errors() <-chan error {
	return make(chan error)
}

func (m *mockPool) Close() error {
	m.closed = true
	return nil
}

var _ pool.Pool = (*mockPool)(nil) // Verify mockPool implements pool.Pool

func TestNew(t *testing.T) {
	// Create mock components
	mocks := newMockComponents()

	// Create test node
	node := &Node{
		network: mocks.network,
		dag:     mocks.dag,
		beacon:  mocks.beacon,
		pool:    mocks.pool,
	}

	// Test configuration validation
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Network: network.Config{
					Port: 0,
				},
				BeaconURL:       "http://example.com",
				BeaconInterval:  time.Second,
				BeaconChainHash: []byte("chain-hash"),
				BeaconPublicKey: []byte("public-key"),
				EventRate:       100,
				AnchorInterval:  5,
				SubEventComplex: 0.3,
				VerifyInterval:  10,
			},
			wantErr: false,
		},
		{
			name: "empty beacon URL",
			cfg: Config{
				Network: network.Config{
					Port: 0,
				},
				BeaconInterval:  time.Second,
				BeaconChainHash: []byte("chain-hash"),
				BeaconPublicKey: []byte("public-key"),
				EventRate:       100,
				AnchorInterval:  5,
				SubEventComplex: 0.3,
				VerifyInterval:  10,
			},
			wantErr: true,
		},
		{
			name: "invalid beacon interval",
			cfg: Config{
				Network: network.Config{
					Port: 0,
				},
				BeaconURL:       "http://example.com",
				BeaconInterval:  time.Millisecond,
				BeaconChainHash: []byte("chain-hash"),
				BeaconPublicKey: []byte("public-key"),
				EventRate:       100,
				AnchorInterval:  5,
				SubEventComplex: 0.3,
				VerifyInterval:  10,
			},
			wantErr: true,
		},
		{
			name: "zero event rate",
			cfg: Config{
				Network: network.Config{
					Port: 0,
				},
				BeaconURL:       "http://example.com",
				BeaconInterval:  time.Second,
				BeaconChainHash: []byte("chain-hash"),
				BeaconPublicKey: []byte("public-key"),
				EventRate:       0,
				AnchorInterval:  5,
				SubEventComplex: 0.3,
				VerifyInterval:  10,
			},
			wantErr: true,
		},
		{
			name: "zero anchor interval",
			cfg: Config{
				Network: network.Config{
					Port: 0,
				},
				BeaconURL:       "http://example.com",
				BeaconInterval:  time.Second,
				BeaconChainHash: []byte("chain-hash"),
				BeaconPublicKey: []byte("public-key"),
				EventRate:       100,
				AnchorInterval:  0,
				SubEventComplex: 0.3,
				VerifyInterval:  10,
			},
			wantErr: true,
		},
		{
			name: "invalid sub-event complexity",
			cfg: Config{
				Network: network.Config{
					Port: 0,
				},
				BeaconURL:       "http://example.com",
				BeaconInterval:  time.Second,
				BeaconChainHash: []byte("chain-hash"),
				BeaconPublicKey: []byte("public-key"),
				EventRate:       100,
				AnchorInterval:  5,
				SubEventComplex: 1.5,
				VerifyInterval:  10,
			},
			wantErr: true,
		},
		{
			name: "zero verify interval",
			cfg: Config{
				Network: network.Config{
					Port: 0,
				},
				BeaconURL:       "http://example.com",
				BeaconInterval:  time.Second,
				BeaconChainHash: []byte("chain-hash"),
				BeaconPublicKey: []byte("public-key"),
				EventRate:       100,
				AnchorInterval:  5,
				SubEventComplex: 0.3,
				VerifyInterval:  0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test configuration validation
			err := ValidateConfig(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	// Test node cleanup
	err := node.Close()
	assert.NoError(t, err)
	assert.True(t, mocks.network.closed)
	assert.True(t, mocks.beacon.stopped)
	assert.True(t, mocks.pool.closed)
}

func TestNodeOperations(t *testing.T) {
	ctx := context.Background()
	mocks := newMockComponents()

	// Create test node
	node := &Node{
		network: mocks.network,
		dag:     mocks.dag,
		beacon:  mocks.beacon,
		pool:    mocks.pool,
	}

	// Test adding events
	_, err := node.AddEvent(ctx, []byte("test1"), nil)
	assert.NoError(t, err)

	_, err = node.AddEvent(ctx, []byte("test2"), nil)
	assert.NoError(t, err)

	// Close the node
	err = node.Close()
	assert.NoError(t, err)

	// Verify components were closed
	assert.True(t, mocks.network.closed)
	assert.True(t, mocks.beacon.stopped)
	assert.True(t, mocks.pool.closed)

	// Adding events after close should fail
	_, err = node.AddEvent(ctx, []byte("test3"), nil)
	assert.Error(t, err)
}
