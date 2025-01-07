package pool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/systemshift/dag-time/pkg/dag"
)

func TestEventParentRelationships(t *testing.T) {
	h1, h2, cleanup := setupTestHosts(t)
	defer cleanup()

	// Create pools
	ctx := context.Background()
	p1, p2 := setupTestPools(t, ctx, h1, h2)
	defer p1.Close()
	defer p2.Close()

	// Wait for pubsub to stabilize
	waitForPubsubConnection(t, ctx, p1, p2, h1, h2)

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
}

func TestSubEventRelationships(t *testing.T) {
	h1, h2, cleanup := setupTestHosts(t)
	defer cleanup()

	// Create pools
	ctx := context.Background()
	p1, p2 := setupTestPools(t, ctx, h1, h2)
	defer p1.Close()
	defer p2.Close()

	// Wait for pubsub to stabilize
	waitForPubsubConnection(t, ctx, p1, p2, h1, h2)

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
}
