// Command dagtime provides a CLI tool for running a DAG-Time node
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/systemshift/dag-time/network"
	"github.com/systemshift/dag-time/node"
)

func main() {
	// Parse flags
	port := flag.Int("port", 0, "Node listen port (0 for random)")
	peer := flag.String("peer", "", "Peer multiaddr to connect to")
	beaconURL := flag.String("drand-url", "https://api.drand.sh", "drand HTTP endpoint URL")
	beaconInterval := flag.Duration("drand-interval", 10*time.Second, "How often to fetch drand beacon")
	eventRate := flag.Int64("event-rate", 5000, "How quickly to generate events (in milliseconds)")
	anchorInterval := flag.Int("anchor-interval", 5, "Events before anchoring to drand beacon")
	subEventComplex := flag.Float64("subevent-complexity", 0.3, "Probability of creating sub-events (0.0-1.0)")
	verifyInterval := flag.Int("verify-interval", 5, "Events between verifications")
	flag.Parse()

	// Create configuration
	cfg := node.Config{
		Network: network.Config{
			Port: *port,
			Peer: *peer,
		},
		BeaconURL:       *beaconURL,
		BeaconInterval:  *beaconInterval,
		EventRate:       *eventRate,
		AnchorInterval:  *anchorInterval,
		SubEventComplex: *subEventComplex,
		VerifyInterval:  *verifyInterval,
	}

	// Validate configuration
	if err := node.ValidateConfig(cfg); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create node
	n, err := node.New(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}
	defer n.Close()

	// Log startup
	log.Printf("DAG-Time node started with configuration:")
	log.Printf("  Port: %d", *port)
	if *peer != "" {
		log.Printf("  Peer: %s", *peer)
	}
	log.Printf("  Beacon URL: %s", *beaconURL)
	log.Printf("  Beacon Interval: %v", *beaconInterval)
	log.Printf("  Event Rate: %dms", *eventRate)
	log.Printf("  Anchor Interval: %d events", *anchorInterval)
	log.Printf("  Sub-Event Complexity: %.2f", *subEventComplex)
	log.Printf("  Verify Interval: %d events", *verifyInterval)

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)
}
