package pool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/systemshift/dag-time/pkg/dag"
)

func TestComplexSubEventRelationships(t *testing.T) {
	h1, h2, cleanup := setupTestHosts(t)
	defer cleanup()

	// Create pools
	ctx := context.Background()
	p1, p2 := setupTestPools(t, ctx, h1, h2)
	defer p1.Close()
	defer p2.Close()

	// Wait for pubsub to stabilize
	waitForPubsubConnection(t, ctx, p1, p2, h1, h2)

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
}

func TestEventChainVerification(t *testing.T) {
	h1, h2, cleanup := setupTestHosts(t)
	defer cleanup()

	// Create pools
	ctx := context.Background()
	p1, p2 := setupTestPools(t, ctx, h1, h2)
	defer p1.Close()
	defer p2.Close()

	// Wait for pubsub to stabilize
	waitForPubsubConnection(t, ctx, p1, p2, h1, h2)

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
}
