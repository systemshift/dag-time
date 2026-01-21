// Example program demonstrating event creation and relationships in DAG-Time
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/systemshift/dag-time/network"
	"github.com/systemshift/dag-time/node"
)

func main() {
	// Create configuration with high sub-event complexity
	cfg := node.Config{
		Network: network.Config{
			Port: 3000,
		},
		BeaconURL:         "https://api.drand.sh",
		BeaconInterval:    10 * time.Second,
		BeaconChainHash:   []byte("8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce"),
		BeaconPublicKey:   []byte("868f005eb8e6e4ca0a47c8a77ceaa5309a47978a7c71bc5cce96366b5d7a569937c529eeda66c7293784bc9402c6bc2d6cd7750008daea6c7d18e75c34b853677747f823"),
		BeaconGenesisTime: 1595431050, // drand mainnet genesis time
		EventRate:         1000,
		AnchorInterval:    5,
		SubEventComplex:   0.8,
		VerifyInterval:    5,
		Verbose:           true,
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

	// Create events periodically
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Add event with some example data
				if _, err := n.AddEvent(ctx, []byte("example-event"), nil); err != nil {
					log.Printf("Failed to add event: %v", err)
				}
			}
		}
	}()

	log.Printf("Creating events every second. Press Ctrl+C to stop.")

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)
}
