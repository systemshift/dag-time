package network

import (
	"context"
	"testing"
	"time"
)

func TestNewNode(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{
			name:    "random port",
			port:    0,
			wantErr: false,
		},
		{
			name:    "specific port",
			port:    3000,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := NewNode(ctx, tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewNode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if node == nil {
				t.Error("NewNode() returned nil node")
				return
			}
			defer node.Close()

			// Verify node has valid ID
			if node.Host.ID() == "" {
				t.Error("Node has empty ID")
			}

			// Verify node has at least one address
			addrs := node.Host.Addrs()
			if len(addrs) == 0 {
				t.Error("Node has no addresses")
			}

			// If specific port was requested, verify it's being used
			if tt.port != 0 {
				found := false
				for _, addr := range addrs {
					if addr.String()[len(addr.String())-4:] == "3000" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Node not listening on requested port %d", tt.port)
				}
			}
		})
	}
}

func TestNodeConnection(t *testing.T) {
	ctx := context.Background()

	// Create two nodes
	node1, err := NewNode(ctx, 0)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	defer node1.Close()

	node2, err := NewNode(ctx, 0)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}
	defer node2.Close()

	// Get node1's address
	addr := node1.Host.Addrs()[0]
	peerAddr := addr.String() + "/p2p/" + node1.Host.ID().String()

	// Connect node2 to node1
	if err := node2.Connect(ctx, peerAddr); err != nil {
		t.Fatalf("Failed to connect nodes: %v", err)
	}

	// Verify connection
	time.Sleep(100 * time.Millisecond) // Give time for connection to establish
	peers := node1.Host.Network().Peers()
	if len(peers) != 1 {
		t.Errorf("Expected 1 peer, got %d", len(peers))
	}
	if peers[0] != node2.Host.ID() {
		t.Error("Connected to wrong peer")
	}
}

func TestInvalidPeerAddress(t *testing.T) {
	ctx := context.Background()

	// Create node
	node, err := NewNode(ctx, 0)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer node.Close()

	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{
			name:    "empty address",
			addr:    "",
			wantErr: true,
		},
		{
			name:    "invalid multiaddr",
			addr:    "not-a-multiaddr",
			wantErr: true,
		},
		{
			name:    "missing peer ID",
			addr:    "/ip4/127.0.0.1/tcp/3000",
			wantErr: true,
		},
		{
			name:    "invalid peer ID",
			addr:    "/ip4/127.0.0.1/tcp/3000/p2p/not-a-peer-id",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := node.Connect(ctx, tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Connect() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPingService(t *testing.T) {
	ctx := context.Background()

	// Create two nodes
	node1, err := NewNode(ctx, 0)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	defer node1.Close()

	node2, err := NewNode(ctx, 0)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}
	defer node2.Close()

	// Connect nodes
	addr := node1.Host.Addrs()[0]
	peerAddr := addr.String() + "/p2p/" + node1.Host.ID().String()
	if err := node2.Connect(ctx, peerAddr); err != nil {
		t.Fatalf("Failed to connect nodes: %v", err)
	}

	// Test ping
	ch := node2.PingService.Ping(ctx, node1.Host.ID())
	select {
	case res := <-ch:
		if res.Error != nil {
			t.Errorf("Ping failed: %v", res.Error)
		}
		if res.RTT == 0 {
			t.Error("Ping returned zero RTT")
		}
	case <-time.After(time.Second):
		t.Error("Ping timed out")
	}
}
