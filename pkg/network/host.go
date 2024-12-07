package network

import (
	"context"
	"fmt"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/multiformats/go-multiaddr"
)

// Node represents a p2p network node
type Node struct {
	Host        host.Host
	PingService *ping.PingService
}

// NewNode creates a new p2p node
func NewNode(ctx context.Context, port int) (*Node, error) {
	// Create multiaddr for local node
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port))
	if err != nil {
		return nil, fmt.Errorf("creating multiaddr: %w", err)
	}

	log.Printf("Creating libp2p node with address: %s", addr)

	// Create libp2p host with debug logging
	h, err := libp2p.New(
		libp2p.ListenAddrs(addr),
		libp2p.Ping(false), // We'll create our own ping service
		libp2p.EnableRelay(),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating host: %w", err)
	}

	// Create ping service
	pingService := ping.NewPingService(h)

	// Create node
	node := &Node{
		Host:        h,
		PingService: pingService,
	}

	// Log node info
	log.Printf("Node created with ID: %s", h.ID())
	log.Printf("Node addresses:")
	for _, addr := range h.Addrs() {
		log.Printf("  %s/p2p/%s", addr, h.ID())
	}

	return node, nil
}

// Connect attempts to connect to a peer
func (n *Node) Connect(ctx context.Context, peerAddr string) error {
	log.Printf("Attempting to connect to peer address: %s", peerAddr)

	// Parse the peer multiaddr
	addr, err := multiaddr.NewMultiaddr(peerAddr)
	if err != nil {
		log.Printf("Error parsing peer address: %v", err)
		return fmt.Errorf("invalid peer address %s: %w", peerAddr, err)
	}
	log.Printf("Parsed multiaddr: %s", addr)

	// Extract peer info from multiaddr
	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		log.Printf("Error extracting peer info: %v", err)
		return fmt.Errorf("getting peer info: %w", err)
	}
	log.Printf("Extracted peer info - ID: %s, Addrs: %v", peerInfo.ID, peerInfo.Addrs)

	// Connect to peer
	log.Printf("Attempting to connect to peer: %s", peerInfo.ID)
	if err := n.Host.Connect(ctx, *peerInfo); err != nil {
		log.Printf("Error connecting to peer: %v", err)
		return fmt.Errorf("connecting to peer: %w", err)
	}

	log.Printf("Successfully connected to peer: %s", peerInfo.ID)
	return nil
}

// Close shuts down the node
func (n *Node) Close() error {
	return n.Host.Close()
}
