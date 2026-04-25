// Command dagtime provides a CLI tool for running a DAG-Time node
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"log/slog"
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
	verbose := flag.Bool("verbose", false, "Enable verbose (debug-level) logging")
	logJSON := flag.Bool("log-json", false, "Emit logs as JSON instead of text")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	handlerOpts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if *logJSON {
		handler = slog.NewJSONHandler(os.Stderr, handlerOpts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, handlerOpts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Parse hex values
	chainHash, err := hex.DecodeString(*beaconChainHash)
	if err != nil {
		logger.Error("invalid chain hash", "err", err)
		os.Exit(1)
	}

	publicKey, err := hex.DecodeString(*beaconPublicKey)
	if err != nil {
		logger.Error("invalid public key", "err", err)
		os.Exit(1)
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
		Verbose:           *verbose,
		Logger:            logger,
	}

	// Validate configuration
	if err := node.ValidateConfig(cfg); err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create node
	n, err := node.New(ctx, cfg)
	if err != nil {
		logger.Error("failed to create node", "err", err)
		os.Exit(1)
	}
	defer n.Close()

	logger.Info("dag-time node started",
		"port", *port,
		"peer", *peer,
		"beacon_url", *beaconURL,
		"beacon_interval", *beaconInterval,
		"beacon_chain_hash", *beaconChainHash,
		"event_rate_ms", *eventRate,
		"anchor_interval", *anchorInterval,
		"subevent_complexity", *subEventComplex,
		"verify_interval", *verifyInterval,
		"verbose", *verbose)

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down", "signal", sig.String())

	// Cancel context to signal goroutines to stop
	cancel()

	// Close node (this will also run via defer, but explicit is clearer)
	if err := n.Close(); err != nil {
		logger.Error("error during shutdown", "err", err)
	}

	logger.Info("shutdown complete")
}
