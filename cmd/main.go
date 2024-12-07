package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/systemshift/dag-time/pkg/beacon"
	"github.com/systemshift/dag-time/pkg/network"
	"github.com/systemshift/dag-time/pkg/pool"
)

var (
	listenPort    = flag.Int("port", 0, "Node listen port (0 for random)")
	connectTo     = flag.String("peer", "", "Peer multiaddr to connect to")
	drandURL      = flag.String("drand-url", "https://api.drand.sh", "drand HTTP endpoint")
	drandInterval = flag.Duration("drand-interval", 10*time.Second, "How often to fetch drand beacon")
	eventInterval = flag.Duration("event-interval", 5*time.Second, "How often to generate events")
)

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("DAG-Time: Starting...")

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
	log.Printf("Creating node on port %d", *listenPort)
	node, err := network.NewNode(ctx, *listenPort)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}
	defer func() {
		log.Printf("Closing node")
		node.Close()
	}()

	// Connect to peer if specified
	if *connectTo != "" {
		log.Printf("Attempting to connect to peer: %s", *connectTo)
		if err := node.Connect(ctx, *connectTo); err != nil {
			log.Fatalf("Failed to connect to peer: %v", err)
		}
		log.Printf("Successfully connected to peer")
	}

	// Initialize drand beacon client
	log.Printf("Initializing drand beacon client")
	b, err := beacon.NewDrandBeacon(*drandURL)
	if err != nil {
		log.Fatalf("Failed to create beacon client: %v", err)
	}

	// Start beacon fetching
	log.Printf("Starting beacon fetching")
	if err := b.Start(ctx, *drandInterval); err != nil {
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
	ticker := time.NewTicker(*eventInterval)
	defer ticker.Stop()

	eventCount := 0
	var lastEventID string // Track last event ID for parent references
	log.Printf("Starting main loop")

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context cancelled")
			log.Println("Shutting down...")
			return

		case round := <-b.Rounds():
			log.Printf("Received beacon round %d", round.Number)
			// If we have a last event, anchor it to this beacon round
			if lastEventID != "" {
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

			// Every 3rd event will be a sub-event of the last event
			if eventCount > 1 && eventCount%3 == 0 && lastEventID != "" {
				err = p.AddSubEvent(ctx, data, lastEventID, nil)
				if err != nil {
					log.Printf("Failed to add sub-event: %v", err)
					continue
				}
			} else {
				err = p.AddEvent(ctx, data, parents)
				if err != nil {
					log.Printf("Failed to add event: %v", err)
					continue
				}
			}

			// Get all events and update lastEventID
			events := p.GetEvents()
			if len(events) > 0 {
				lastEventID = events[len(events)-1].ID
			}

			// Log pool state
			peers := p.GetPeers()
			log.Printf("Pool state: %d events, %d peers", len(events), len(peers))

			// Log peer IDs
			log.Printf("Connected peers:")
			for id := range peers {
				log.Printf("  %s", id)
			}

			// Verify the event chain periodically
			if eventCount%5 == 0 && lastEventID != "" {
				if err := p.VerifyEvent(lastEventID); err != nil {
					log.Printf("Event chain verification failed: %v", err)
				} else {
					log.Printf("Event chain verification successful")
				}
			}
		}
	}
}
