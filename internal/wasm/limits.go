package wasm

import "time"

// Limits on what a guest, or a command it runs, may consume. They are variables so tests can
// shrink them; nothing else changes them.
var (
	// GuestTimeout bounds the time a guest spends computing in one call. Time the host spends
	// waiting for the Captain or for a command it ran for the guest is not counted.
	GuestTimeout = 30 * time.Second

	// DefaultCommandTimeout bounds a command the guest ran when the manifest sets no timeout.
	DefaultCommandTimeout = 5 * time.Minute

	// MaxCommandOutput bounds what the host collects from a command before it kills it.
	MaxCommandOutput = 4 << 20
)
