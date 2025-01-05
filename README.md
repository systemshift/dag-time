# DAG-Time: Integrating a Global Clock with Local Event Streams


## Overview

DAG-Time is a trusted time source—powered by drand—with a local Directed Acyclic Graph (DAG) of events. The goal is to provide a verifiable timeline for fast-evolving local event streams while periodically anchoring them to a well-known global clock for trust and auditability.

## What Problem Does This Solve?

1. **No Global Clock in Distributed Systems**: Many distributed systems (like IPFS) provide content-addressed storage but lack a built-in notion of time. Applications often require a reliable, tamper-evident timeline for events.

2. **Multi-Level Timeline Structure**:
   - **Global Timeline** (Slow, Trustworthy): drand provides publicly available, cryptographically verifiable randomness and a reference timestamp every ~30 seconds (configurable). This gives you a "main spine" or "anchor" for time.
   - **Local Timelines** (Fast, Application-Specific): Applications may generate events (e.g., trades, sensor readings, messages) at a very high rate—milliseconds or faster. Individually, these events don't have a global timestamp, just local ordering.
   - **Sub-Event Networks** (Complex, Interconnected): Each local event can spawn its own network of sub-events, creating deeper hierarchies. These sub-events can form complex relationships by connecting to other sub-events across different parent events, as long as they maintain the acyclic property. This enables rich representation of nested processes, parallel workflows, and intricate dependencies.

By periodically referencing the global drand beacon within the local event DAG, you create a multi-layered structure. The DAG's flexibility allows for complex relationships where:
- Sub-events can branch off from any event
- Sub-events can connect to other sub-events from different parent events
- Multiple sub-event chains can merge back into higher-level events
- Any event at any level can reference the global drand beacon

This creates a rich fabric of interconnected events while maintaining temporal consistency through periodic global anchoring.

## Architecture

1. **Global Anchor (drand Beacon)**:
   - Fetch the latest drand round's randomness and signature at set intervals (e.g., every 10 seconds).
   - Store this beacon's hash as a trusted "checkpoint."

2. **Local DAG of Events**:
   - Each event is hashed along with references to one or more previous events to form a DAG.
   - Events can spawn sub-events, creating deeper hierarchical structures.
   - Sub-events can form connections to other sub-events across different parent events.
   - The DAG can branch and merge at any level, accommodating complex event relationships.
   - Periodically, any event node in the DAG (at any level) can include a pointer (hash) to the most recent drand beacon.

3. **Verification**:  
   Anyone verifying the timeline:
   - Checks the authenticity of the drand beacon (using drand's public keys and verification logic).
   - Ensures that the local DAG node referencing that beacon is correctly hashed and connected to the rest of the DAG.
   - Verifies the integrity of sub-event relationships and their connections across the graph.
   - Concludes that all preceding events in the DAG (including sub-events) happened before or by the time of that drand reference, thereby giving them a temporal ordering without relying solely on the local application's self-issued timestamps.

## Prerequisites

1. **Go Environment**:  
   You'll need Go installed (version 1.18+ recommended).

2. **drand Client**:  
   The demo uses drand's public test beacon. No additional setup is required if you rely on their public endpoints. If you want to run your own drand network, refer to drand's documentation.

## Installation

1. **Clone the Repository**:
   ```bash
   git clone https://github.com/systemshift/dag-time.git
   cd dag-time
   ```

2. **Build the Project**:
   ```bash
   go build -o dag-time cmd/main.go
   ```

3. **(Optional) Running Tests**:
   ```bash
   go test ./...
   ```

## Usage

1. **Start the Demo**:
   ```bash
   ./dag-time
   ```

The demo will:
- Fetch the latest drand randomness periodically.
- Generate a sequence of synthetic local events at a faster rate.
- Create sub-events with complex interconnections.
- Construct a multi-level DAG of events and anchor it to the drand beacon at chosen intervals.

**Configuration Options**:

Network Settings:
- `--port`: Node listen port (0 for random)
- `--peer`: Peer multiaddr to connect to (optional)

Beacon Settings:
- `--drand-url`: The URL of the drand HTTP endpoint (defaults to https://api.drand.sh)
- `--drand-interval`: How often to fetch a new drand beacon (minimum 1s, default 10s)

Event Generation Settings:
- `--event-rate`: How quickly to generate events (minimum 1ms, default 5s)
- `--anchor-interval`: Number of events before anchoring to drand beacon (minimum 1, default 5)
- `--subevent-complexity`: Probability of creating sub-events (0.0-1.0, default 0.3)
- `--verify-interval`: How often to verify event chain integrity (in number of events, minimum 1, default 5)

Example:
```bash
./dag-time \
  --event-rate=100ms \
  --anchor-interval=10 \
  --subevent-complexity=0.7 \
  --verify-interval=20 \
  --drand-interval=5s
```

The above example will:
- Generate events every 100ms
- Anchor every 10th event to the drand beacon
- Create sub-events with 70% probability
- Verify the event chain every 20 events
- Fetch new drand beacons every 5 seconds

2. **Inspecting Outputs**:
   - The console output will log:
     - Newly fetched drand beacons (round number, randomness, signature).
     - Newly created local DAG events and their sub-events (with their hashes).
     - Cross-connections between sub-events.
     - Events that anchor to the global beacon.
   - In a more advanced setup, you could:
     - Print out a serialized DAG (e.g., in JSON) for inspection.
     - Visualize the complex event relationships using graph visualization tools.
     - Store DAG nodes in IPFS or a local database.

## Example Workflow

1. The demo fetches a drand beacon (e.g., round #123456), which yields a randomness value and a timestamp.
2. Your application generates local events E1, E2, E3... every few milliseconds.
3. Each event may spawn sub-events (SE1.1, SE1.2, SE2.1...) that can connect to other sub-events (e.g., SE1.2 → SE2.1).
4. Events at any level can create an "anchor event" E_Anchor that references the most recent drand hash.
5. Verifiers who see E_Anchor (and the known drand beacon) trust that all events before E_Anchor (including sub-events and their cross-connections) occurred before that drand round.

## Extending the Demo

1. **Multiple DAG Branches**:  
   Add branching logic to simulate parallel event streams merging back into a single chain.

2. **Persistence and Retrieval**:  
   Integrate with your preferred storage system to persist DAG nodes and enable offline verification.

3. **Different Time Sources**:  
   Swap out drand for another global anchor (e.g., Bitcoin block headers) to compare trust models.

## Security and Trust Considerations

1. **drand Trust Model**:  
   Drand randomness is produced by a set of distributed participants running a threshold signature scheme. It's designed to be unbiased and unpredictable. Validate its trust assumptions against your requirements.

2. **Local Event Integrity**:  
   Ensure that each event's data is hashed properly. Using cryptographic hash references ensures tamper-evident integrity.

3. **Sub-Event Relationship Verification**:
   Implement careful validation of sub-event relationships to prevent cycles and maintain the DAG properties.

4. **DoS and Scalability**:  
   Consider how often you fetch drand beacons and how large/complex your DAG becomes. You may need caching or efficient verification strategies for production use.
