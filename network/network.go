// Package network handles P2P communication between nodes
package network

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/multiformats/go-multiaddr"
)

// Config represents network configuration
type Config struct {
	// Port is the port to listen on (0 for random)
	Port int

	// Peer is the multiaddr of a peer to connect to (optional)
	Peer string
}

// Node represents a P2P network node
type Node interface {
	// Host returns the libp2p host
	Host() host.Host

	// Connect connects to a peer
	Connect(ctx context.Context, addr multiaddr.Multiaddr) error

	// Peers returns the list of connected peers
	Peers() []peer.ID

	// Close closes the node
	Close() error
}

// node implements the Node interface
type node struct {
	host host.Host
}

// ErrInvalidConfig indicates the network configuration is invalid
var ErrInvalidConfig = fmt.Errorf("invalid network configuration")

// NewNode creates a new P2P network node
func NewNode(ctx context.Context, cfg Config) (Node, error) {
	// Validate port
	if cfg.Port < 0 {
		return nil, fmt.Errorf("%w: port must be non-negative", ErrInvalidConfig)
	}

	// Create libp2p host
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.Port)),
		libp2p.Ping(false),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create host: %w", err)
	}

	// Create node
	n := &node{
		host: h,
	}

	// Enable ping protocol
	ping.NewPingService(h)

	// Connect to peer if specified
	if cfg.Peer != "" {
		addr, err := multiaddr.NewMultiaddr(cfg.Peer)
		if err != nil {
			return nil, fmt.Errorf("invalid peer address: %w", err)
		}

		if err := n.Connect(ctx, addr); err != nil {
			return nil, fmt.Errorf("failed to connect to peer: %w", err)
		}
	}

	return n, nil
}

func (n *node) Host() host.Host {
	return n.host
}

func (n *node) Connect(ctx context.Context, addr multiaddr.Multiaddr) error {
	// Parse peer info from multiaddr
	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("invalid peer address: %w", err)
	}

	// Connect to peer
	if err := n.host.Connect(ctx, *info); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	return nil
}

func (n *node) Peers() []peer.ID {
	return n.host.Network().Peers()
}

func (n *node) Close() error {
	return n.host.Close()
}
