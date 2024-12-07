package pool_test

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/systemshift/dag-time/pkg/dag"
	"github.com/systemshift/dag-time/pkg/pool"
)

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

	// Wait for connection to establish
	time.Sleep(time.Second)

	t.Run("Basic Event Creation and Propagation", func(t *testing.T) {
		// Add event to first pool
		data := []byte("test event")
		err := p1.AddEvent(ctx, data, nil)
		require.NoError(t, err)

		// Wait for propagation
		time.Sleep(time.Second)

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

		// Wait for propagation
		time.Sleep(time.Second)

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

		// Wait for propagation
		time.Sleep(time.Second)

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

		// Wait for propagation
		time.Sleep(time.Second)

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

		// Wait for propagation
		time.Sleep(time.Second)

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

	t.Run("Event Chain Verification", func(t *testing.T) {
		// Create first event in chain
		data1 := []byte("event 1")
		err := p1.AddEvent(ctx, data1, nil)
		require.NoError(t, err)

		// Wait for propagation
		time.Sleep(time.Second)

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

		// Wait for propagation
		time.Sleep(time.Second)

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

		// Wait for propagation
		time.Sleep(time.Second)

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
