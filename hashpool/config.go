// Package hashpool provides integration between hashpool's P2P hash collection
// and dag-time's DAG with drand anchoring.
package hashpool

import (
	"time"
)

// HashpoolConfig contains hashpool-specific settings
type HashpoolConfig struct {
	ListenPort     int
	BootstrapPeers []string
	Difficulty     uint8
	RoundInterval  time.Duration
}

// Config holds adapter configuration
type Config struct {
	// Hashpool network settings
	Hashpool HashpoolConfig

	// IncludeFullHashes includes all hashes in the event data
	IncludeFullHashes bool

	// ChainCommitments links commitments sequentially as DAG events
	ChainCommitments bool

	// Verbose enables detailed logging
	Verbose bool
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() Config {
	return Config{
		Hashpool: HashpoolConfig{
			ListenPort:    0, // Random port
			Difficulty:    16,
			RoundInterval: 10 * time.Second,
		},
		IncludeFullHashes: false,
		ChainCommitments:  true,
		Verbose:           false,
	}
}
