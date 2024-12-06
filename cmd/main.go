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
	"github.com/systemshift/dag-time/pkg/dag"
)

var (
	drandURL        = flag.String("drand-url", "https://api.drand.sh", "drand HTTP endpoint")
	drandInterval   = flag.Duration("drand-interval", 10*time.Second, "How often to fetch drand beacon")
	eventRate       = flag.Duration("event-rate", 100*time.Millisecond, "How often to generate events")
	anchorInterval  = flag.Int("anchor-interval", 10, "How many events before anchoring to drand")
	subEventComplex = flag.Float64("subevent-complexity", 0.3, "Probability of creating sub-events (0.0-1.0)")
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
		<-sigChan
		log.Println("Received shutdown signal")
		cancel()
	}()

	// Initialize drand beacon client
	b, err := beacon.NewDrandBeacon(*drandURL)
	if err != nil {
		log.Fatalf("Failed to create beacon client: %v", err)
	}

	// Start beacon fetching
	if err := b.Start(ctx, *drandInterval); err != nil {
		log.Fatalf("Failed to start beacon: %v", err)
	}
	defer b.Stop()

	// Create DAG
	d := dag.New()

	// Create initial root event
	root, err := dag.NewEvent([]byte("root"), nil)
	if err != nil {
		log.Fatalf("Failed to create root event: %v", err)
	}

	if err := d.AddEvent(root); err != nil {
		log.Fatalf("Failed to add root event: %v", err)
	}

	log.Printf("Created root event: %s", root.ID)

	// Track the latest event for parent references
	latestEvent := root
	eventCount := 0

	// Main event generation loop
	ticker := time.NewTicker(*eventRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down...")
			return

		case round := <-b.Rounds():
			log.Printf("Received beacon round %d", round.Number)

		case <-ticker.C:
			eventCount++
			var event *dag.Event
			var err error

			// Decide if this should be a regular event or sub-event
			if eventCount > 1 && *subEventComplex > 0 && randFloat() < *subEventComplex {
				// Create a sub-event
				event, err = dag.NewSubEvent(
					[]byte(fmt.Sprintf("sub-event-%d", eventCount)),
					latestEvent.ID,
					nil,
				)
			} else {
				// Create a regular event
				event, err = dag.NewEvent(
					[]byte(fmt.Sprintf("event-%d", eventCount)),
					[]string{latestEvent.ID},
				)
			}

			if err != nil {
				log.Printf("Failed to create event: %v", err)
				continue
			}

			// Every anchorInterval events, fetch latest round and anchor
			if eventCount%*anchorInterval == 0 {
				round, err := b.GetLatestRound(ctx)
				if err != nil {
					log.Printf("Failed to get latest round: %v", err)
				} else {
					event.SetBeaconRound(round.Number, round.Randomness)
					log.Printf("Anchored event %s to beacon round %d", event.ID, round.Number)
				}
			}

			if err := d.AddEvent(event); err != nil {
				log.Printf("Failed to add event: %v", err)
				continue
			}

			latestEvent = event
			log.Printf("Created %s: %s", eventTypeStr(event), event.ID)
		}
	}
}

func eventTypeStr(e *dag.Event) string {
	if e.IsSubEvent {
		return "sub-event"
	}
	return "event"
}

func randFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}
