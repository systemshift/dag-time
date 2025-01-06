package pool_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"strings"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/systemshift/dag-time/pkg/dag"
	"github.com/systemshift/dag-time/pkg/pool"
)

// waitForEvents waits for a pool to have at least count events
func waitForEvents(p *pool.Pool, count int) error {
	timeout := time.After(10 * time.Second)
	start := time.Now()
	for {
		select {
		case <-timeout:
			events := p.GetEvents()
			return fmt.Errorf("timeout after %v waiting for %d events, got %d events: %v",
				time.Since(start), count, len(events),
				formatEvents(events))
		default:
			events := p.GetEvents()
			if len(events) >= count {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// formatEvents returns a string representation of events for debugging
func formatEvents(events []*dag.Event) string {
	var result []string
	for _, e := range events {
		result = append(result, fmt.Sprintf("{ID: %s, Data: %q}", e.ID, e.Data))
	}
	return fmt.Sprintf("[%s]", strings.Join(result, ", "))
}

func TestPool(t *testing.T) {
	// Create libp2p hosts with TCP transport
	h1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	defer h1.Close()

	h2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	defer h2.Close()

	// Create pools
	ctx := context.Background()
	p1, err := pool.NewPool(ctx, h1)
	require.NoError(t, err)
	defer p1.Close()

	p2, err := pool.NewPool(ctx, h2)
	require.NoError(t, err)
	defer p2.Close()

	// Get h2's multiaddr
	h2Addr := h2.Addrs()[0]
	h2Info := peer.AddrInfo{
		ID:    h2.ID(),
		Addrs: []multiaddr.Multiaddr{h2Addr},
	}

	// Connect h1 to h2
	err = h1.Connect(ctx, h2Info)
	require.NoError(t, err)

	// Create context with longer timeout for pubsub setup
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Wait for connection and pubsub to establish
	connected := make(chan struct{})
	go func() {
		for {
			// Check both network connection and pubsub peers
			if len(h1.Network().Peers()) > 0 && len(p1.GetPeers()) > 0 {
				// Wait a bit more for pubsub to fully establish
				time.Sleep(time.Second)
				close(connected)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case <-ctx.Done():
		t.Fatal("Timeout waiting for connection")
	case <-connected:
		t.Logf("Connection established - Network peers: %d, PubSub peers: %d",
			len(h1.Network().Peers()), len(p1.GetPeers()))
	}

	t.Run("Basic Event Creation and Propagation", func(t *testing.T) {
		// Add event to first pool
		data := []byte("test event")
		err := p1.AddEvent(ctx, data, nil)
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, 1))

		// Check events in both pools
		events1 := p1.GetEvents()
		events2 := p2.GetEvents()

		assert.Equal(t, 1, len(events1))
		assert.Equal(t, 1, len(events2))
		assert.Equal(t, events1[0].ID, events2[0].ID)
		assert.Equal(t, data, events1[0].Data)
	})

	t.Run("Event Parent Relationships", func(t *testing.T) {
		// Create first event
		data1 := []byte("parent event")
		err := p1.AddEvent(ctx, data1, nil)
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Get the parent event ID
		events := p1.GetEvents()
		var parentID string
		for _, e := range events {
			if string(e.Data) == "parent event" {
				parentID = e.ID
				break
			}
		}
		require.NotEmpty(t, parentID)

		// Create child event with explicit parent reference
		data2 := []byte("child event")
		err = p1.AddEvent(ctx, data2, []string{parentID})
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Get the child event
		events = p1.GetEvents()
		var childEvent *dag.Event
		for _, e := range events {
			if string(e.Data) == "child event" {
				childEvent = e
				break
			}
		}
		require.NotNil(t, childEvent)

		// Verify parent relationship
		assert.Contains(t, childEvent.Parents, parentID)
	})

	t.Run("Sub-Event Creation and Relationships", func(t *testing.T) {
		// Create parent event
		parentData := []byte("parent event for sub-event")
		err := p1.AddEvent(ctx, parentData, nil)
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Get the parent event ID
		events := p1.GetEvents()
		var parentID string
		for _, e := range events {
			if string(e.Data) == "parent event for sub-event" {
				parentID = e.ID
				break
			}
		}
		require.NotEmpty(t, parentID)

		// Create sub-event
		subEventData := []byte("sub event")
		err = p1.AddSubEvent(ctx, subEventData, parentID, nil)
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Verify relationships
		parentEvent, err := p1.GetEvent(parentID)
		require.NoError(t, err)

		// Find the sub-event
		events = p1.GetEvents()
		var subEvent *dag.Event
		for _, e := range events {
			if string(e.Data) == "sub event" {
				subEvent = e
				break
			}
		}
		require.NotNil(t, subEvent)

		assert.True(t, subEvent.IsSubEvent)
		assert.Equal(t, parentID, subEvent.ParentEvent)
		assert.Contains(t, parentEvent.SubEvents, subEvent.ID)
	})

	t.Run("Complex Sub-Event Relationships", func(t *testing.T) {
		// Create two parent events
		parent1Data := []byte("parent event 1")
		err := p1.AddEvent(ctx, parent1Data, nil)
		require.NoError(t, err)

		parent2Data := []byte("parent event 2")
		err = p1.AddEvent(ctx, parent2Data, nil)
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Get parent event IDs
		events := p1.GetEvents()
		var parent1ID, parent2ID string
		for _, e := range events {
			if string(e.Data) == "parent event 1" {
				parent1ID = e.ID
			}
			if string(e.Data) == "parent event 2" {
				parent2ID = e.ID
			}
		}
		require.NotEmpty(t, parent1ID)
		require.NotEmpty(t, parent2ID)

		// Create sub-events for both parents
		subEvent1Data := []byte("sub event 1")
		err = p1.AddSubEvent(ctx, subEvent1Data, parent1ID, nil)
		require.NoError(t, err)

		subEvent2Data := []byte("sub event 2")
		err = p1.AddSubEvent(ctx, subEvent2Data, parent2ID, nil)
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Find sub-event IDs
		events = p1.GetEvents()
		var subEvent1ID, subEvent2ID string
		for _, e := range events {
			if string(e.Data) == "sub event 1" {
				subEvent1ID = e.ID
			}
			if string(e.Data) == "sub event 2" {
				subEvent2ID = e.ID
			}
		}
		require.NotEmpty(t, subEvent1ID)
		require.NotEmpty(t, subEvent2ID)

		// Create a sub-event that connects to both previous sub-events
		crossConnectData := []byte("cross connected sub-event")
		err = p1.AddSubEvent(ctx, crossConnectData, parent1ID, []string{subEvent2ID})
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Find the cross-connected event
		events = p1.GetEvents()
		var crossEvent *dag.Event
		for _, e := range events {
			if string(e.Data) == "cross connected sub-event" {
				crossEvent = e
				break
			}
		}
		require.NotNil(t, crossEvent)

		// Verify relationships
		assert.True(t, crossEvent.IsSubEvent)
		assert.Equal(t, parent1ID, crossEvent.ParentEvent)
		assert.Contains(t, crossEvent.Parents, subEvent2ID)

		// Verify the entire structure
		err = p1.VerifyEvent(crossEvent.ID)
		assert.NoError(t, err)

		// Verify propagation to second pool
		events2 := p2.GetEvents()
		var crossEventInPool2 *dag.Event
		for _, e := range events2 {
			if string(e.Data) == "cross connected sub-event" {
				crossEventInPool2 = e
				break
			}
		}
		require.NotNil(t, crossEventInPool2)
		assert.Equal(t, crossEvent.ID, crossEventInPool2.ID)
	})

	t.Run("Event Chain Verification", func(t *testing.T) {
		// Create first event in chain
		data1 := []byte("event 1")
		err := p1.AddEvent(ctx, data1, nil)
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Get event1 ID
		events := p1.GetEvents()
		var event1ID string
		for _, e := range events {
			if string(e.Data) == "event 1" {
				event1ID = e.ID
				break
			}
		}
		require.NotEmpty(t, event1ID)

		// Create second event referencing first
		data2 := []byte("event 2")
		err = p1.AddEvent(ctx, data2, []string{event1ID})
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Get event2 ID
		events = p1.GetEvents()
		var event2ID string
		for _, e := range events {
			if string(e.Data) == "event 2" {
				event2ID = e.ID
				break
			}
		}
		require.NotEmpty(t, event2ID)

		// Create sub-event of event2
		subEventData := []byte("sub event of event 2")
		err = p1.AddSubEvent(ctx, subEventData, event2ID, nil)
		require.NoError(t, err)

		require.NoError(t, waitForEvents(p2, len(p1.GetEvents())))

		// Get sub-event ID
		events = p1.GetEvents()
		var subEventID string
		for _, e := range events {
			if string(e.Data) == "sub event of event 2" {
				subEventID = e.ID
				break
			}
		}
		require.NotEmpty(t, subEventID)

		// Verify the entire chain
		err = p1.VerifyEvent(subEventID)
		assert.NoError(t, err)
	})

	t.Run("Peer Management", func(t *testing.T) {
		peers1 := p1.GetPeers()
		peers2 := p2.GetPeers()

		assert.Equal(t, 1, len(peers1))
		assert.Equal(t, 1, len(peers2))
		assert.Contains(t, peers1, h2.ID())
		assert.Contains(t, peers2, h1.ID())
	})
}
