package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/systemshift/dag-time/pkg/beacon"
	"github.com/systemshift/dag-time/pkg/network"
	"github.com/systemshift/dag-time/pkg/pool"
)

type config struct {
	// Network settings
	listenPort int
	connectTo  string

	// Beacon settings
	drandURL      string
	drandInterval time.Duration

	// Event generation settings
	eventRate          time.Duration
	anchorInterval     int
	subEventComplexity float64
	verifyInterval     int
}

func parseFlags() (*config, error) {
	cfg := &config{}

	// Network flags
	flag.IntVar(&cfg.listenPort, "port", 0, "Node listen port (0 for random)")
	flag.StringVar(&cfg.connectTo, "peer", "", "Peer multiaddr to connect to")

	// Beacon flags
	flag.StringVar(&cfg.drandURL, "drand-url", "https://api.drand.sh", "drand HTTP endpoint")
	flag.DurationVar(&cfg.drandInterval, "drand-interval", 10*time.Second, "How often to fetch drand beacon")

	// Event generation flags
	flag.DurationVar(&cfg.eventRate, "event-rate", 5*time.Second, "How quickly to generate events")
	flag.IntVar(&cfg.anchorInterval, "anchor-interval", 5, "How many events before anchoring to drand beacon")
	flag.Float64Var(&cfg.subEventComplexity, "subevent-complexity", 0.3, "Probability of creating sub-events (0.0-1.0)")
	flag.IntVar(&cfg.verifyInterval, "verify-interval", 5, "How often to verify event chain (in number of events)")

	flag.Parse()

	// Validate configuration
	if cfg.subEventComplexity < 0 || cfg.subEventComplexity > 1 {
		return nil, fmt.Errorf("subevent-complexity must be between 0.0 and 1.0")
	}
	if cfg.anchorInterval < 1 {
		return nil, fmt.Errorf("anchor-interval must be greater than 0")
	}
	if cfg.verifyInterval < 1 {
		return nil, fmt.Errorf("verify-interval must be greater than 0")
	}
	if cfg.eventRate < time.Millisecond {
		return nil, fmt.Errorf("event-rate must be at least 1ms")
	}
	if cfg.drandInterval < time.Second {
		return nil, fmt.Errorf("drand-interval must be at least 1s")
	}

	return cfg, nil
}

func main() {
	// Configure logging
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("DAG-Time: Starting...")

	// Parse and validate configuration
	cfg, err := parseFlags()
	if err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v", sig)
		cancel()
	}()

	// Create p2p node
	log.Printf("Creating node on port %d", cfg.listenPort)
	node, err := network.NewNode(ctx, cfg.listenPort)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}
	defer func() {
		log.Printf("Closing node")
		node.Close()
	}()

	// Connect to peer if specified
	if cfg.connectTo != "" {
		log.Printf("Attempting to connect to peer: %s", cfg.connectTo)
		if err := node.Connect(ctx, cfg.connectTo); err != nil {
			log.Fatalf("Failed to connect to peer: %v", err)
		}
		log.Printf("Successfully connected to peer")
	}

	// Initialize drand beacon client
	log.Printf("Initializing drand beacon client")
	b, err := beacon.NewDrandBeacon(cfg.drandURL)
	if err != nil {
		log.Fatalf("Failed to create beacon client: %v", err)
	}

	// Start beacon fetching
	log.Printf("Starting beacon fetching")
	if err := b.Start(ctx, cfg.drandInterval); err != nil {
		log.Fatalf("Failed to start beacon: %v", err)
	}
	defer func() {
		log.Printf("Stopping beacon")
		b.Stop()
	}()

	// Create event pool
	log.Printf("Creating event pool")
	p, err := pool.NewPool(ctx, node.Host)
	if err != nil {
		log.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		log.Printf("Closing pool")
		p.Close()
	}()

	// Start event generation
	ticker := time.NewTicker(cfg.eventRate)
	defer ticker.Stop()

	eventCount := 0
	var lastEventID string // Track last event ID for parent references
	log.Printf("Starting main loop with settings:")
	log.Printf("  Event rate: %v", cfg.eventRate)
	log.Printf("  Anchor interval: %d events", cfg.anchorInterval)
	log.Printf("  Sub-event complexity: %.2f", cfg.subEventComplexity)
	log.Printf("  Verify interval: %d events", cfg.verifyInterval)

	// Initialize random source (not needed for rand/v2 as it's automatically seeded)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context cancelled")
			log.Println("Shutting down...")
			return

		case round := <-b.Rounds():
			log.Printf("Received beacon round %d", round.Number)
			// Anchor to beacon based on anchor interval
			if lastEventID != "" && eventCount%cfg.anchorInterval == 0 {
				if event, err := p.GetEvent(lastEventID); err == nil {
					event.SetBeaconRound(round.Number, round.Randomness)
					log.Printf("Anchored event %s to beacon round %d", lastEventID, round.Number)
				}
			}

		case <-ticker.C:
			eventCount++
			data := []byte(fmt.Sprintf("test event %d", eventCount))

			var parents []string
			if lastEventID != "" {
				parents = []string{lastEventID}
			}

			// Create sub-event based on complexity setting
			if eventCount > 1 && lastEventID != "" && rand.Float64() < float64(cfg.subEventComplexity) {
				err = p.AddSubEvent(ctx, data, lastEventID, nil)
				if err != nil {
					log.Printf("Failed to add sub-event: %v", err)
					continue
				}
				log.Printf("Added sub-event with parent %s", lastEventID)
			} else {
				err = p.AddEvent(ctx, data, parents)
				if err != nil {
					log.Printf("Failed to add event: %v", err)
					continue
				}
				log.Printf("Added regular event")
			}

			// Get all events and update lastEventID
			events := p.GetEvents()
			if len(events) > 0 {
				lastEventID = events[len(events)-1].ID
			}

			// Log pool state
			peers := p.GetPeers()
			log.Printf("Pool state: %d events, %d peers", len(events), len(peers))

			// Log peer IDs at a lower frequency to reduce noise
			if eventCount%10 == 0 {
				log.Printf("Connected peers:")
				for id := range peers {
					log.Printf("  %s", id)
				}
			}

			// Verify the event chain periodically
			if eventCount%cfg.verifyInterval == 0 && lastEventID != "" {
				if err := p.VerifyEvent(lastEventID); err != nil {
					log.Printf("Event chain verification failed: %v", err)
				} else {
					log.Printf("Event chain verification successful")
				}
			}
		}
	}
}
