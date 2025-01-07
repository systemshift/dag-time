package pool

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/stretchr/testify/require"
	"github.com/systemshift/dag-time/pkg/dag"
	"github.com/systemshift/dag-time/pkg/pool"
)

// waitForEvents waits for a pool to have at least count events
func waitForEvents(p *pool.Pool, count int) error {
	timeout := time.After(10 * time.Second)
	start := time.Now()
	for {
		select {
		case <-timeout:
			events := p.GetEvents()
			return fmt.Errorf("timeout after %v waiting for %d events, got %d events: %v",
				time.Since(start), count, len(events),
				formatEvents(events))
		default:
			events := p.GetEvents()
			if len(events) >= count {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// formatEvents returns a string representation of events for debugging
func formatEvents(events []*dag.Event) string {
	result := make([]string, 0, len(events))
	for _, e := range events {
		result = append(result, fmt.Sprintf("{ID: %s, Data: %q}", e.ID, e.Data))
	}
	return fmt.Sprintf("[%s]", strings.Join(result, ", "))
}

func setupTestHosts(t *testing.T) (h1, h2 host.Host, cleanup func()) {
	h1tmp, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)

	h2tmp, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)

	return h1tmp, h2tmp, func() {
		h1tmp.Close()
		h2tmp.Close()
	}
}

func setupTestPools(t *testing.T, ctx context.Context, h1, h2 host.Host) (*pool.Pool, *pool.Pool) {
	p1, err := pool.NewPool(ctx, h1)
	require.NoError(t, err)

	p2, err := pool.NewPool(ctx, h2)
	require.NoError(t, err)

	return p1, p2
}

func waitForPubsubConnection(t *testing.T, ctx context.Context, p1, p2 *pool.Pool, h1, h2 host.Host) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Wait for network connection
	connected := make(chan struct{})
	go func() {
		for {
			if len(h1.Network().Peers()) > 0 {
				close(connected)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case <-ctx.Done():
		t.Fatal("Timeout waiting for network connection")
	case <-connected:
		t.Logf("Network connection established - Peers: %d", len(h1.Network().Peers()))
	}

	// Wait for peers to see each other in topic
	t.Log("Waiting for p1 to see p2 in topic")
	err := p1.WaitForTopicPeer(ctx, h2.ID())
	require.NoError(t, err)

	t.Log("Waiting for p2 to see p1 in topic")
	err = p2.WaitForTopicPeer(ctx, h1.ID())
	require.NoError(t, err)

	t.Logf("Topic peers - p1: %v, p2: %v", p1.GetTopicPeers(), p2.GetTopicPeers())

	// Wait for pubsub to fully stabilize
	time.Sleep(5 * time.Second)
	t.Log("Pubsub stabilization period complete")
}
