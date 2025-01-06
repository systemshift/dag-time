// Package dagtime provides the main library interface for DAG-Time
package dagtime

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/systemshift/dag-time/pkg/beacon"
	"github.com/systemshift/dag-time/pkg/dag"
	"github.com/systemshift/dag-time/pkg/pool"
)

// Node represents a DAG-Time node with all its components
type Node struct {
	Pool   *pool.Pool
	DAG    *dag.DAG
	Beacon *beacon.DrandBeacon
}

// NewNode creates a new DAG-Time node with the given libp2p host and drand URL
func NewNode(ctx context.Context, h host.Host, drandURL string) (*Node, error) {
	// Create DAG
	d := dag.NewDAG()

	// Create pool
	p, err := pool.NewPool(ctx, h)
	if err != nil {
		return nil, err
	}

	// Create beacon
	b, err := beacon.NewDrandBeacon(drandURL)
	if err != nil {
		p.Close()
		return nil, err
	}

	return &Node{
		Pool:   p,
		DAG:    d,
		Beacon: b,
	}, nil
}

// Close shuts down the node and all its components
func (n *Node) Close() error {
	n.Beacon.Stop() // drand beacon stop is idempotent, error can be ignored
	if err := n.Pool.Close(); err != nil {
		return fmt.Errorf("closing pool: %w", err)
	}
	return nil
}
