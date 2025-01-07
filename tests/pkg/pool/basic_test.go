package pool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicEventCreation(t *testing.T) {
	h1, h2, cleanup := setupTestHosts(t)
	defer cleanup()

	// Create pools
	ctx := context.Background()
	p1, p2 := setupTestPools(t, ctx, h1, h2)
	defer p1.Close()
	defer p2.Close()

	// Wait for pubsub to stabilize
	waitForPubsubConnection(t, ctx, p1, p2, h1, h2)

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
}
