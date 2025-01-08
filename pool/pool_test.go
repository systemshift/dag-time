package pool

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/systemshift/dag-time/beacon"
	"github.com/systemshift/dag-time/dag"
)

// mockHost implements a minimal host.Host for testing
type mockHost struct {
	host.Host // Embed to get default implementations
}

func (m *mockHost) ID() peer.ID  { return "" }
func (m *mockHost) Close() error { return nil }

// mockDAG implements a minimal dag.DAG for testing
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

func (m *mockDAG) Verify(ctx context.Context) error {
	return nil
}

// mockBeacon implements a minimal beacon.Beacon for testing
type mockBeacon struct {
	roundCh chan *beacon.Round
}

func newMockBeacon() *mockBeacon {
	return &mockBeacon{
		roundCh: make(chan *beacon.Round, 1),
	}
}

func (m *mockBeacon) GetLatestRound(ctx context.Context) (*beacon.Round, error) {
	return &beacon.Round{Number: 1}, nil
}

func (m *mockBeacon) GetRound(ctx context.Context, round uint64) (*beacon.Round, error) {
	return &beacon.Round{Number: round}, nil
}

func (m *mockBeacon) Start(ctx context.Context, interval time.Duration) error {
	return nil
}

func (m *mockBeacon) Stop() error {
	return nil
}

func (m *mockBeacon) Subscribe() (<-chan *beacon.Round, error) {
	return m.roundCh, nil
}

func TestNewPool(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				EventRate:       100,
				AnchorInterval:  5,
				SubEventComplex: 0.3,
				VerifyInterval:  10,
			},
			wantErr: false,
		},
		{
			name: "zero event rate",
			cfg: Config{
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
			ctx := context.Background()
			h := &mockHost{}
			d := newMockDAG()
			b := newMockBeacon()

			p, err := NewPool(ctx, h, d, b, tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, p)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}

func TestPoolOperations(t *testing.T) {
	ctx := context.Background()
	h := &mockHost{}
	d := newMockDAG()
	b := newMockBeacon()

	cfg := Config{
		EventRate:       100,
		AnchorInterval:  2,
		SubEventComplex: 0.3,
		VerifyInterval:  5,
	}

	p, err := NewPool(ctx, h, d, b, cfg)
	require.NoError(t, err)
	require.NotNil(t, p)

	// Test adding events
	err = p.AddEvent(ctx, []byte("test1"), nil)
	assert.NoError(t, err)

	err = p.AddEvent(ctx, []byte("test2"), nil)
	assert.NoError(t, err)

	// Send a beacon round
	b.roundCh <- &beacon.Round{
		Number:     123,
		Randomness: []byte("random"),
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Close the pool
	err = p.Close()
	assert.NoError(t, err)

	// Adding events after close should fail
	err = p.AddEvent(ctx, []byte("test3"), nil)
	assert.Error(t, err)
}
