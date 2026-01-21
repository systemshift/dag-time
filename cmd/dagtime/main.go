// Command dagtime provides a CLI tool for running a DAG-Time node
package main

import (
	"context"
	"encoding/hex"
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
	// Default drand values from League of Entropy mainnet
	defaultChainHash := "8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce"
	defaultPublicKey := "868f005eb8e6e4ca0a47c8a77ceaa5309a47978a7c71bc5cce96366b5d7a569937c529eeda66c7293784bc9402c6bc2d6cd7750008daea6c7d18e75c34b853677747f823"
	defaultGenesisTime := int64(1595431050) // drand mainnet genesis time

	// Parse flags
	port := flag.Int("port", 0, "Node listen port (0 for random)")
	peer := flag.String("peer", "", "Peer multiaddr to connect to")
	beaconURL := flag.String("drand-url", "https://api.drand.sh", "drand HTTP endpoint URL")
	beaconInterval := flag.Duration("drand-interval", 10*time.Second, "How often to fetch drand beacon")
	beaconChainHash := flag.String("drand-chain-hash", defaultChainHash, "drand chain hash (hex)")
	beaconPublicKey := flag.String("drand-public-key", defaultPublicKey, "drand public key (hex)")
	beaconGenesisTime := flag.Int64("drand-genesis-time", defaultGenesisTime, "drand genesis time (unix timestamp)")
	eventRate := flag.Int64("event-rate", 5000, "How quickly to generate events (in milliseconds)")
	anchorInterval := flag.Int("anchor-interval", 5, "Events before anchoring to drand beacon")
	subEventComplex := flag.Float64("subevent-complexity", 0.3, "Probability of creating sub-events and cross-event relationships (0.0-1.0)")
	verifyInterval := flag.Int("verify-interval", 5, "Events between verifications")
	verbose := flag.Bool("verbose", false, "Enable verbose logging of event relationships")
	flag.Parse()

	// Parse hex values
	chainHash, err := hex.DecodeString(*beaconChainHash)
	if err != nil {
		log.Fatalf("Invalid chain hash: %v", err)
	}

	publicKey, err := hex.DecodeString(*beaconPublicKey)
	if err != nil {
		log.Fatalf("Invalid public key: %v", err)
	}

	// Create configuration
	cfg := node.Config{
		Network: network.Config{
			Port: *port,
			Peer: *peer,
		},
		BeaconURL:         *beaconURL,
		BeaconInterval:    *beaconInterval,
		BeaconChainHash:   chainHash,
		BeaconPublicKey:   publicKey,
		BeaconGenesisTime: *beaconGenesisTime,
		EventRate:         *eventRate,
		AnchorInterval:    *anchorInterval,
		SubEventComplex:   *subEventComplex,
		VerifyInterval:    *verifyInterval,
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
	log.Printf("  Beacon Chain Hash: %s", *beaconChainHash)
	log.Printf("  Event Rate: %dms", *eventRate)
	log.Printf("  Anchor Interval: %d events", *anchorInterval)
	log.Printf("  Sub-Event Complexity: %.2f", *subEventComplex)
	log.Printf("  Verify Interval: %d events", *verifyInterval)
	log.Printf("  Verbose Logging: %v", *verbose)

	if *verbose {
		log.Printf("\nEvent relationships will be logged as they are created.")
		log.Printf("- Main events are top-level events in the DAG")
		log.Printf("- Sub-events are spawned by main events")
		log.Printf("- Sub-events can connect to other sub-events")
		log.Printf("- Higher complexity means more sub-events and connections")
	}

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)

	// Cancel context to signal goroutines to stop
	cancel()

	// Close node (this will also run via defer, but explicit is clearer)
	if err := n.Close(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Printf("Shutdown complete")
}
