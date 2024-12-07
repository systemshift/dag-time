package pool

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
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

func waitForPeerConnection(t *testing.T, h1, h2 host.Host) {
	// Wait for the connection to be established
	for i := 0; i < 50; i++ { // Try for 5 seconds max
		if h1.Network().Connectedness(h2.ID()) == network.Connected &&
			h2.Network().Connectedness(h1.ID()) == network.Connected {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Peers failed to connect within timeout")
}

func waitForEvent(t *testing.T, p *Pool, eventID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events := p.GetEvents()
		for _, e := range events {
			if e.ID == eventID {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Event not received within timeout")
}

func waitForPubSubReady(t *testing.T, p1, p2 *Pool) {
	// Wait for pubsub to establish
	deadline := time.Now().Add(10 * time.Second)
	testEvent := &PoolEvent{
		ID:        "test-sync-event",
		Data:      []byte("sync"),
		Timestamp: time.Now(),
		Creator:   p1.host.ID().String(),
	}

	for time.Now().Before(deadline) {
		// Try to send a test event
		if err := p1.AddEvent(context.Background(), testEvent); err != nil {
			t.Logf("Failed to send test event: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Check if p2 received it
		events := p2.GetEvents()
		for _, e := range events {
			if e.ID == testEvent.ID {
				return // PubSub is working
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("PubSub failed to establish within timeout")
}

func TestNewPool(t *testing.T) {
	ctx := context.Background()
	h1, h2 := setupTestHosts(t)
	defer h2.Close()
	defer h1.Close()

	waitForPeerConnection(t, h1, h2)

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

	// Wait for pubsub to establish
	waitForPubSubReady(t, p1, p2)

	// Verify pools are empty (excluding test sync event)
	events1 := p1.GetEvents()
	events2 := p2.GetEvents()
	if len(events1) > 1 || len(events2) > 1 {
		t.Error("Pools should only contain sync event")
	}
}

func TestEventPropagation(t *testing.T) {
	ctx := context.Background()
	h1, h2 := setupTestHosts(t)
	defer h2.Close()
	defer h1.Close()

	waitForPeerConnection(t, h1, h2)

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

	// Wait for pubsub to establish
	waitForPubSubReady(t, p1, p2)

	// Create and add event to pool1
	event := &PoolEvent{
		ID:        "test-propagation-event",
		Data:      []byte("test data"),
		Timestamp: time.Now(),
		Creator:   h1.ID().String(),
	}

	if err := p1.AddEvent(ctx, event); err != nil {
		t.Fatalf("Failed to add event: %v", err)
	}

	// Wait for event to propagate to pool2
	waitForEvent(t, p2, event.ID, 5*time.Second)

	// Verify event was received correctly
	events := p2.GetEvents()
	var found bool
	for _, e := range events {
		if e.ID == event.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Event not found in pool2")
	}
}

func TestPeerTracking(t *testing.T) {
	ctx := context.Background()
	h1, h2 := setupTestHosts(t)
	defer h2.Close()
	defer h1.Close()

	waitForPeerConnection(t, h1, h2)

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

	// Wait for pubsub to establish
	waitForPubSubReady(t, p1, p2)

	// Create and add event to pool2
	event := &PoolEvent{
		ID:        "test-tracking-event",
		Data:      []byte("test data"),
		Timestamp: time.Now(),
		Creator:   h2.ID().String(),
	}

	if err := p2.AddEvent(ctx, event); err != nil {
		t.Fatalf("Failed to add event: %v", err)
	}

	// Wait for event to propagate
	waitForEvent(t, p1, event.ID, 5*time.Second)

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
