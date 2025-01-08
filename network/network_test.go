package network

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNode(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Port: 0, // random port
			},
			wantErr: false,
		},
		{
			name: "negative port",
			cfg: Config{
				Port: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid peer address",
			cfg: Config{
				Port: 0,
				Peer: "invalid-addr",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			node, err := NewNode(ctx, tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, node)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
				assert.NoError(t, node.Close())
			}
		})
	}
}

func TestNodeConnections(t *testing.T) {
	ctx := context.Background()

	// Create first node
	node1, err := NewNode(ctx, Config{Port: 0})
	require.NoError(t, err)
	defer node1.Close()

	// Get node1's address
	addr := node1.Host().Addrs()[0]
	peerID := node1.Host().ID()
	fullAddr := addr.String() + "/p2p/" + peerID.String()

	// Create second node and connect to first
	node2, err := NewNode(ctx, Config{
		Port: 0,
		Peer: fullAddr,
	})
	require.NoError(t, err)
	defer node2.Close()

	// Wait for connection to establish
	time.Sleep(100 * time.Millisecond)

	// Check connections
	assert.Equal(t, 1, len(node1.Peers()))
	assert.Equal(t, 1, len(node2.Peers()))
	assert.Equal(t, node2.Host().ID(), node1.Peers()[0])
	assert.Equal(t, node1.Host().ID(), node2.Peers()[0])

	// Test connecting to invalid address
	err = node2.Connect(ctx, addr) // Missing peer ID
	assert.Error(t, err)
}

func TestNodeClose(t *testing.T) {
	ctx := context.Background()

	// Create and immediately close a node
	node, err := NewNode(ctx, Config{Port: 0})
	require.NoError(t, err)
	require.NoError(t, node.Close())

	// Create two connected nodes
	node1, err := NewNode(ctx, Config{Port: 0})
	require.NoError(t, err)

	addr := node1.Host().Addrs()[0]
	peerID := node1.Host().ID()
	fullAddr := addr.String() + "/p2p/" + peerID.String()

	node2, err := NewNode(ctx, Config{
		Port: 0,
		Peer: fullAddr,
	})
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Close first node
	require.NoError(t, node1.Close())

	// Wait for disconnect
	time.Sleep(100 * time.Millisecond)

	// Check node2 has no peers
	assert.Equal(t, 0, len(node2.Peers()))

	// Clean up
	require.NoError(t, node2.Close())
}
