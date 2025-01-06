package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/systemshift/dag-time/pkg/dag"
)

const (
	TopicName = "dag-time-pool"
)

// Pool manages the temporary event pool
type Pool struct {
	mu     sync.RWMutex
	host   host.Host
	pubsub *pubsub.PubSub
	topic  *pubsub.Topic
	sub    *pubsub.Subscription
	dag    *dag.DAG
	peers  map[peer.ID]time.Time
	cancel context.CancelFunc
	done   chan struct{}
	closed bool
}

// GetHost returns the libp2p host for testing purposes
func (p *Pool) GetHost() host.Host {
	return p.host
}

// NewPool creates a new temporary event pool
func NewPool(ctx context.Context, h host.Host) (*Pool, error) {
	log.Printf("Creating new pool with host ID: %s", h.ID())

	// Create pubsub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub: %w", err)
	}
	log.Printf("Created GossipSub instance")

	// Join topic
	topic, err := ps.Join(TopicName)
	if err != nil {
		return nil, fmt.Errorf("joining topic: %w", err)
	}
	log.Printf("Joined topic: %s", TopicName)

	// Subscribe to topic
	sub, err := topic.Subscribe()
	if err != nil {
		topic.Close()
		return nil, fmt.Errorf("subscribing to topic: %w", err)
	}
	log.Printf("Subscribed to topic")

	// Create cancellable context for message handler
	handlerCtx, cancel := context.WithCancel(ctx)

	pool := &Pool{
		host:   h,
		pubsub: ps,
		topic:  topic,
		sub:    sub,
		dag:    dag.NewDAG(),
		peers:  make(map[peer.ID]time.Time),
		cancel: cancel,
		done:   make(chan struct{}),
	}

	// Start handling messages
	go pool.handleMessages(handlerCtx)
	log.Printf("Started message handler")

	// Start peer tracking
	go pool.trackPeers(handlerCtx)

	return pool, nil
}

// trackPeers periodically updates the peer list
func (p *Pool) trackPeers(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.Lock()
			if !p.closed {
				// Get all connected peers from the host
				for _, peerID := range p.host.Network().Peers() {
					p.peers[peerID] = time.Now()
				}
			}
			p.mu.Unlock()
		}
	}
}

// handleMessages processes incoming messages from the pubsub
func (p *Pool) handleMessages(ctx context.Context) {
	defer close(p.done)
	log.Printf("Message handler started for host %s", p.host.ID())

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context cancelled stopping message handler")
			return
		default:
			msg, err := p.sub.Next(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Error getting next message: %v", err)
				continue
			}

			// Skip messages from ourselves
			if msg.ReceivedFrom == p.host.ID() {
				log.Printf("Skipping message from self")
				continue
			}

			// Update peer last seen time
			p.mu.Lock()
			if !p.closed {
				p.peers[msg.ReceivedFrom] = time.Now()
			}
			p.mu.Unlock()

			// Decode event
			var event dag.Event
			if err := json.Unmarshal(msg.Data, &event); err != nil {
				log.Printf("Error decoding message: %v", err)
				continue
			}

			// Add event to DAG
			p.mu.Lock()
			if !p.closed {
				if err := p.dag.AddEvent(&event); err != nil {
					log.Printf("Error adding event to DAG: %v", err)
					continue
				}
			}
			p.mu.Unlock()

			log.Printf("Received event %s from peer %s", event.ID, msg.ReceivedFrom)
		}
	}
}

// AddEvent adds a new event to the pool and broadcasts it
func (p *Pool) AddEvent(ctx context.Context, data []byte, parents []string) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("pool is closed")
	}
	p.mu.RUnlock()

	// Create new event
	event, err := dag.NewEvent(data, parents)
	if err != nil {
		return fmt.Errorf("creating event: %w", err)
	}

	log.Printf("Adding event %s", event.ID)

	// Add event to DAG
	p.mu.Lock()
	if err := p.dag.AddEvent(event); err != nil {
		p.mu.Unlock()
		return fmt.Errorf("adding event to DAG: %w", err)
	}
	p.mu.Unlock()

	// Broadcast event
	data, err = json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encoding event: %w", err)
	}

	if err := p.topic.Publish(ctx, data); err != nil {
		return fmt.Errorf("publishing event: %w", err)
	}

	log.Printf("Published event %s to topic", event.ID)
	return nil
}

// findRecentSubEvents finds sub-events within a given time window
func (p *Pool) findRecentSubEvents(timeWindow time.Duration) []*dag.Event {
	p.mu.RLock()
	defer p.mu.RUnlock()

	cutoff := time.Now().Add(-timeWindow)
	var recentSubEvents []*dag.Event

	events := p.dag.GetAllEvents()
	for _, event := range events {
		if event.IsSubEvent && event.Timestamp.After(cutoff) {
			recentSubEvents = append(recentSubEvents, event)
		}
	}

	return recentSubEvents
}

// selectAdditionalParents randomly selects valid additional parents from recent sub-events
func (p *Pool) selectAdditionalParents(recentSubEvents []*dag.Event, parentEventID string, maxAdditional int) []string {
	if len(recentSubEvents) == 0 {
		return nil
	}

	if maxAdditional > len(recentSubEvents) {
		maxAdditional = len(recentSubEvents)
	}

	// Create shuffled indices
	indices := make([]int, len(recentSubEvents))
	for i := range indices {
		indices[i] = i
	}
	rand.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})

	// Select valid parents
	additionalParents := make([]string, 0, maxAdditional)
	for _, idx := range indices {
		if len(additionalParents) >= maxAdditional {
			break
		}

		subEvent := recentSubEvents[idx]
		if subEvent.ID == parentEventID || p.isDescendantOf(subEvent.ID, parentEventID) {
			continue
		}

		additionalParents = append(additionalParents, subEvent.ID)
	}

	return additionalParents
}

// isDescendantOf checks if eventID is a descendant of ancestorID
func (p *Pool) isDescendantOf(eventID, ancestorID string) bool {
	visited := make(map[string]bool)
	var checkAncestors func(string) bool
	checkAncestors = func(id string) bool {
		if visited[id] {
			return false
		}
		visited[id] = true

		event, err := p.GetEvent(id)
		if err != nil {
			return false
		}

		if event.ID == ancestorID {
			return true
		}

		for _, parentID := range event.Parents {
			if checkAncestors(parentID) {
				return true
			}
		}
		return false
	}

	return checkAncestors(eventID)
}

// AddSubEvent adds a new sub-event to the pool with optional connections to other sub-events
func (p *Pool) AddSubEvent(ctx context.Context, data []byte, parentEventID string, additionalParents []string) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("pool is closed")
	}

	// Find recent sub-events that could be potential parents
	recentSubEvents := p.findRecentSubEvents(30 * time.Second)
	p.mu.RUnlock()

	// Select additional parents if none provided
	if len(additionalParents) == 0 {
		additionalParents = p.selectAdditionalParents(recentSubEvents, parentEventID, 2)
	}

	// Create and add the sub-event
	event, err := dag.NewSubEvent(data, parentEventID, additionalParents)
	if err != nil {
		return fmt.Errorf("creating sub-event: %w", err)
	}

	log.Printf("Adding sub-event %s with parent %s and %d additional connections",
		event.ID, parentEventID, len(additionalParents))

	// Add event to DAG
	p.mu.Lock()
	if err := p.dag.AddEvent(event); err != nil {
		p.mu.Unlock()
		return fmt.Errorf("adding sub-event to DAG: %w", err)
	}
	p.mu.Unlock()

	// Broadcast event
	data, err = json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encoding sub-event: %w", err)
	}

	if err := p.topic.Publish(ctx, data); err != nil {
		return fmt.Errorf("publishing sub-event: %w", err)
	}

	log.Printf("Published sub-event %s to topic", event.ID)
	return nil
}

// GetEvents returns all events in the pool
func (p *Pool) GetEvents() []*dag.Event {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.dag.GetAllEvents()
}

// GetEvent retrieves a specific event by ID
func (p *Pool) GetEvent(id string) (*dag.Event, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.dag.GetEvent(id)
}

// GetPeers returns all known peers and their last seen time
func (p *Pool) GetPeers() map[peer.ID]time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()

	peers := make(map[peer.ID]time.Time, len(p.peers))
	for id, lastSeen := range p.peers {
		peers[id] = lastSeen
	}
	return peers
}

// VerifyEvent verifies an event and its chain of ancestors
func (p *Pool) VerifyEvent(eventID string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.dag.VerifyEventChain(eventID)
}

// Close shuts down the pool
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// Cancel message handler and wait for it to finish
	p.cancel()
	<-p.done

	// Clean up in reverse order of creation
	p.sub.Cancel()
	if err := p.topic.Close(); err != nil {
		return fmt.Errorf("closing topic: %w", err)
	}
	return nil
}
