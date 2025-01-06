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

	"github.com/systemshift/dag-time/pkg/dagtime"
	"github.com/systemshift/dag-time/pkg/network"
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

// setupNode creates and configures the network and DAG-Time nodes
func setupNode(ctx context.Context, cfg *config) (*network.Node, *dagtime.Node, error) {
	// Create p2p node
	log.Printf("Creating node on port %d", cfg.listenPort)
	netNode, err := network.NewNode(ctx, cfg.listenPort)
	if err != nil {
		return nil, nil, fmt.Errorf("creating network node: %w", err)
	}

	// Connect to peer if specified
	if cfg.connectTo != "" {
		log.Printf("Attempting to connect to peer: %s", cfg.connectTo)
		if err := netNode.Connect(ctx, cfg.connectTo); err != nil {
			netNode.Close()
			return nil, nil, fmt.Errorf("connecting to peer: %w", err)
		}
		log.Printf("Successfully connected to peer")
	}

	// Create DAG-Time node
	log.Printf("Creating DAG-Time node")
	node, err := dagtime.NewNode(ctx, netNode.Host, cfg.drandURL)
	if err != nil {
		netNode.Close()
		return nil, nil, fmt.Errorf("creating DAG-Time node: %w", err)
	}

	// Start beacon fetching
	log.Printf("Starting beacon fetching")
	if err := node.Beacon.Start(ctx, cfg.drandInterval); err != nil {
		node.Close()
		netNode.Close()
		return nil, nil, fmt.Errorf("starting beacon: %w", err)
	}

	return netNode, node, nil
}

// beaconRound represents a drand beacon round
type beaconRound struct {
	Number     uint64
	Randomness []byte
}

// handleBeaconRound processes a new beacon round
func handleBeaconRound(node *dagtime.Node, round *beaconRound, eventCount int, lastEventID string, anchorInterval int) {
	log.Printf("Received beacon round %d", round.Number)
	if lastEventID != "" && eventCount%anchorInterval == 0 {
		if event, err := node.Pool.GetEvent(lastEventID); err == nil {
			event.SetBeaconRound(round.Number, round.Randomness)
			log.Printf("Anchored event %s to beacon round %d", lastEventID, round.Number)
		}
	}
}

// generateEvent creates a new event or sub-event
func generateEvent(ctx context.Context, node *dagtime.Node, eventCount int, lastEventID string, subEventComplexity float64) error {
	data := []byte(fmt.Sprintf("test event %d", eventCount))
	var parents []string
	if lastEventID != "" {
		parents = []string{lastEventID}
	}

	if eventCount > 1 && lastEventID != "" && rand.Float64() < subEventComplexity {
		if err := node.Pool.AddSubEvent(ctx, data, lastEventID, nil); err != nil {
			return fmt.Errorf("adding sub-event: %w", err)
		}
		log.Printf("Added sub-event with parent %s", lastEventID)
	} else {
		if err := node.Pool.AddEvent(ctx, data, parents); err != nil {
			return fmt.Errorf("adding event: %w", err)
		}
		log.Printf("Added regular event")
	}
	return nil
}

// logPoolState logs the current state of the event pool
func logPoolState(node *dagtime.Node, eventCount int) string {
	events := node.Pool.GetEvents()
	peers := node.Pool.GetPeers()
	log.Printf("Pool state: %d events, %d peers", len(events), len(peers))

	// Log peer IDs at a lower frequency
	if eventCount%10 == 0 {
		log.Printf("Connected peers:")
		for id := range peers {
			log.Printf("  %s", id)
		}
	}

	// Return last event ID
	if len(events) > 0 {
		return events[len(events)-1].ID
	}
	return ""
}

// verifyEventChain performs periodic verification of the event chain
func verifyEventChain(node *dagtime.Node, eventCount int, lastEventID string, verifyInterval int) {
	if eventCount%verifyInterval == 0 && lastEventID != "" {
		if err := node.Pool.VerifyEvent(lastEventID); err != nil {
			log.Printf("Event chain verification failed: %v", err)
		} else {
			log.Printf("Event chain verification successful")
		}
	}
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

	// Set up nodes
	netNode, node, err := setupNode(ctx, cfg)
	if err != nil {
		log.Fatalf("Setup failed: %v", err)
	}
	defer func() {
		log.Printf("Closing nodes")
		node.Close()
		netNode.Close()
	}()

	// Start event generation
	ticker := time.NewTicker(cfg.eventRate)
	defer ticker.Stop()

	eventCount := 0
	var lastEventID string
	log.Printf("Starting main loop with settings:")
	log.Printf("  Event rate: %v", cfg.eventRate)
	log.Printf("  Anchor interval: %d events", cfg.anchorInterval)
	log.Printf("  Sub-event complexity: %.2f", cfg.subEventComplexity)
	log.Printf("  Verify interval: %d events", cfg.verifyInterval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context cancelled")
			log.Println("Shutting down...")
			return

		case round := <-node.Beacon.Rounds():
			// Convert beacon round to local type
			localRound := &beaconRound{
				Number:     round.Number,
				Randomness: round.Randomness,
			}
			handleBeaconRound(node, localRound, eventCount, lastEventID, cfg.anchorInterval)

		case <-ticker.C:
			eventCount++
			if err := generateEvent(ctx, node, eventCount, lastEventID, cfg.subEventComplexity); err != nil {
				log.Printf("Failed to generate event: %v", err)
				continue
			}

			lastEventID = logPoolState(node, eventCount)
			verifyEventChain(node, eventCount, lastEventID, cfg.verifyInterval)
		}
	}
}
