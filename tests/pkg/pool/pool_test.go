package pool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPeerManagement(t *testing.T) {
	h1, h2, cleanup := setupTestHosts(t)
	defer cleanup()

	// Create pools
	ctx := context.Background()
	p1, p2 := setupTestPools(t, ctx, h1, h2)
	defer p1.Close()
	defer p2.Close()

	// Wait for pubsub to stabilize
	waitForPubsubConnection(t, ctx, p1, p2, h1, h2)

	peers1 := p1.GetPeers()
	peers2 := p2.GetPeers()

	assert.Equal(t, 1, len(peers1))
	assert.Equal(t, 1, len(peers2))
	assert.Contains(t, peers1, h2.ID())
	assert.Contains(t, peers2, h1.ID())
}
