package pool

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
)

func setupTestHosts(t *testing.T) (host.Host, host.Host) {
	// Create two hosts for testing
	h1, err := libp2p.New()
	if err != nil {
		t.Fatalf("Failed to create host1: %v", err)
	}

	h2, err := libp2p.New()
	if err != nil {
		t.Fatalf("Failed to create host2: %v", err)
	}

	// Connect the hosts
	h2info := host.InfoFromHost(h2)
	if err := h1.Connect(context.Background(), *h2info); err != nil {
		t.Fatalf("Failed to connect hosts: %v", err)
	}

	return h1, h2
}

func TestNewPool(t *testing.T) {
	ctx := context.Background()
	h1, h2 := setupTestHosts(t)
	defer h1.Close()
	defer h2.Close()

	// Create pools
	p1, err := NewPool(ctx, h1)
	if err != nil {
		t.Fatalf("Failed to create pool1: %v", err)
	}
	defer p1.Close()

	p2, err := NewPool(ctx, h2)
	if err != nil {
		t.Fatalf("Failed to create pool2: %v", err)
	}
	defer p2.Close()

	// Verify pools are empty
	if len(p1.GetEvents()) != 0 {
		t.Error("New pool should be empty")
	}
	if len(p2.GetEvents()) != 0 {
		t.Error("New pool should be empty")
	}
}

func TestEventPropagation(t *testing.T) {
	ctx := context.Background()
	h1, h2 := setupTestHosts(t)
	defer h1.Close()
	defer h2.Close()

	// Create pools
	p1, err := NewPool(ctx, h1)
	if err != nil {
		t.Fatalf("Failed to create pool1: %v", err)
	}
	defer p1.Close()

	p2, err := NewPool(ctx, h2)
	if err != nil {
		t.Fatalf("Failed to create pool2: %v", err)
	}
	defer p2.Close()

	// Give time for pubsub to establish
	time.Sleep(time.Second)

	// Create and add event to pool1
	event := &PoolEvent{
		ID:        "test-event",
		Data:      []byte("test data"),
		Timestamp: time.Now(),
		Creator:   h1.ID().String(),
	}

	if err := p1.AddEvent(ctx, event); err != nil {
		t.Fatalf("Failed to add event: %v", err)
	}

	// Wait for event to propagate
	time.Sleep(time.Second)

	// Verify event was received by pool2
	events := p2.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event in pool2, got %d", len(events))
	}
	if events[0].ID != event.ID {
		t.Error("Received wrong event")
	}
}

func TestPeerTracking(t *testing.T) {
	ctx := context.Background()
	h1, h2 := setupTestHosts(t)
	defer h1.Close()
	defer h2.Close()

	// Create pools
	p1, err := NewPool(ctx, h1)
	if err != nil {
		t.Fatalf("Failed to create pool1: %v", err)
	}
	defer p1.Close()

	p2, err := NewPool(ctx, h2)
	if err != nil {
		t.Fatalf("Failed to create pool2: %v", err)
	}
	defer p2.Close()

	// Give time for pubsub to establish
	time.Sleep(time.Second)

	// Create and add event to pool2
	event := &PoolEvent{
		ID:        "test-event",
		Data:      []byte("test data"),
		Timestamp: time.Now(),
		Creator:   h2.ID().String(),
	}

	if err := p2.AddEvent(ctx, event); err != nil {
		t.Fatalf("Failed to add event: %v", err)
	}

	// Wait for event to propagate
	time.Sleep(time.Second)

	// Verify peer tracking in pool1
	peers := p1.GetPeers()
	if len(peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(peers))
	}

	// Verify the peer is h2
	found := false
	for id := range peers {
		if id == h2.ID() {
			found = true
			break
		}
	}
	if !found {
		t.Error("Peer tracking failed to record correct peer")
	}
}

func TestEventValidation(t *testing.T) {
	ctx := context.Background()
	h1, _ := setupTestHosts(t)
	defer h1.Close()

	p1, err := NewPool(ctx, h1)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer p1.Close()

	tests := []struct {
		name    string
		event   *PoolEvent
		wantErr bool
	}{
		{
			name: "valid event",
			event: &PoolEvent{
				ID:        "test-event",
				Data:      []byte("test data"),
				Timestamp: time.Now(),
				Creator:   h1.ID().String(),
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			event: &PoolEvent{
				Data:      []byte("test data"),
				Timestamp: time.Now(),
				Creator:   h1.ID().String(),
			},
			wantErr: true,
		},
		{
			name: "missing creator",
			event: &PoolEvent{
				ID:        "test-event",
				Data:      []byte("test data"),
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p1.AddEvent(ctx, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
