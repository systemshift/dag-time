// Package hashpool provides integration between hashpool's P2P hash collection
// and dag-time's DAG with drand anchoring.
package hashpool

import (
	"log/slog"
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

	// Verbose enables detailed logging.
	// When Logger is nil, Verbose=true installs a Debug-level stderr handler.
	// When Logger is non-nil, Verbose is ignored.
	Verbose bool

	// Logger is the structured logger used by the adapter. Nil uses
	// slog.Default() (Verbose may bump it to Debug — see above).
	Logger *slog.Logger
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
