# DAG-Time Architecture

## Overview
DAG-Time is a trusted time source—powered by drand—with a local Directed Acyclic Graph (DAG) of events. The system provides a verifiable timeline for fast-evolving local event streams while periodically anchoring them to a well-known global clock for trust and auditability.

### Problem Statement
1. **No Global Clock in Distributed Systems**: Many distributed systems (like IPFS) provide content-addressed storage but lack a built-in notion of time. Applications often require a reliable, tamper-evident timeline for events.

2. **Multi-Level Timeline Structure**:
   - **Global Timeline** (Slow, Trustworthy): drand provides publicly available, cryptographically verifiable randomness and a reference timestamp every ~30 seconds.
   - **Local Timelines** (Fast, Application-Specific): Applications may generate events at a very high rate—milliseconds or faster.
   - **Sub-Event Networks** (Complex, Interconnected): Each local event can spawn its own network of sub-events, creating deeper hierarchies with complex relationships.

## Core Components

### 1. Network Layer (`network` package)
- Built on libp2p for P2P communication
- Components:
  - `Node`: Manages P2P networking and connections
  - Features:
    - Dynamic port binding
    - Peer discovery and connection
    - Host management

### 2. DAG-Time Core
The core functionality is split across several packages:

#### High-Level Interface (`dagtime` package)
- Provides complete node implementation
- Integrates all components
- Simplified API for common operations

#### Low-Level Components
- `dag` package: Core DAG implementation
- `pool` package: Event pool management
- `beacon` package: drand beacon integration

#### Features
- Event creation and management
- Sub-event handling
- Event chain verification
- Beacon round anchoring

### 3. Event System
#### Event Types
- Regular Events
  - Contains data payload
  - Links to parent events
  - Can be anchored to beacon rounds
- Sub-Events
  - Linked to parent events
  - Creates complex event relationships
  - Maintains DAG structure

#### Event Properties
- Unique identification (ID)
- Parent references
- Data payload
- Optional beacon anchoring
  - Round number
  - Randomness data

### 4. Time Synchronization
- Uses drand for distributed randomness and time synchronization
- Features:
  - Periodic beacon fetching
  - Event anchoring to beacon rounds
  - Configurable intervals for synchronization

### 5. Configuration System
Key configurable parameters:
- Network settings
  - Listen port
  - Peer connections
- Beacon settings
  - drand URL
  - Beacon fetch interval
- Event generation
  - Event rate
  - Anchor interval
  - Sub-event complexity
  - Verification interval

## System Flow

1. **Initialization**
   - Parse configuration
   - Set up network node
   - Initialize DAG-Time node
   - Start beacon fetching

2. **Runtime Operation**
   - Event Generation
     - Create regular events or sub-events
     - Link events to parents
     - Maintain event pool
   
   - Beacon Processing
     - Fetch beacon rounds
     - Anchor events to beacons
     - Maintain time synchronization

   - Verification
     - Periodic event chain verification
     - Maintain DAG integrity
     - Peer state monitoring

3. **State Management**
   - Event pool maintenance
   - Peer connection tracking
   - Chain verification
   - System metrics logging

## Dependencies

### Core Dependencies
- `github.com/libp2p/go-libp2p`: P2P networking
- `github.com/libp2p/go-libp2p-pubsub`: PubSub functionality
- `github.com/multiformats/go-multiaddr`: Network addressing

### Testing Dependencies
- `github.com/stretchr/testify`: Testing framework

## Security and Trust Considerations

1. **drand Trust Model**
   - Distributed randomness production via threshold signature scheme
   - Designed for unbiased and unpredictable output
   - Trust assumptions should be validated against requirements

2. **Event Integrity**
   - Cryptographic hash references ensure tamper-evident integrity
   - Proper event data hashing validation
   - Sub-event relationship verification to maintain DAG properties

3. **Operational Considerations**
   - DoS prevention strategies
   - Beacon fetching optimization
   - DAG size and complexity management
   - Caching and efficient verification strategies

## Future Considerations

1. **Scalability**
   - Event propagation optimization
   - Network topology management
   - Resource usage optimization
   - Multiple DAG branches support
   - Efficient parallel event stream merging

2. **Security**
   - Event validation mechanisms
   - Peer authentication
   - Anti-spam measures
   - Alternative time source integration (e.g., Bitcoin block headers)

3. **Reliability**
   - Event persistence
   - Recovery mechanisms
   - Network partition handling
   - Storage system integration
   - Offline verification support

4. **Monitoring**
   - Performance metrics
   - Network health monitoring
   - Event chain analytics
   - DAG complexity metrics

## Implementation Notes

The system is designed to be:
- Distributed: No central coordination
- Asynchronous: Event-driven architecture
- Scalable: P2P network based
- Verifiable: Regular chain verification
- Configurable: Extensive parameter tuning
