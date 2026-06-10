package models

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

const epochMs = int64(1735689600000) // 2025-01-01 00:00:00 UTC

// Entity type prefix constants — 2-char codes embedded in every OrbitID.
const (
	EntityTypeVessel      = "vs"
	EntityTypeGalaxy      = "gx"
	EntityTypeSolarSystem = "sy"
	EntityTypePlanet      = "pl"
	EntityTypeCallsign    = "cs"
	EntityTypeTransponder = "tp"
	EntityTypeResource    = "rs"
	EntityTypeDefault     = "df"
	EntityTypeBeacon      = "bk"
	EntityTypeNavHistory  = "nh"
)

// OrbitID is the parsed form of a 16-character OrbitID string.
type OrbitID struct {
	Raw        string
	EntityType string
	Timestamp  time.Time
}

// NewID generates a new 16-character OrbitID for the given entity type prefix.
// Format: [8 base36 timestamp][2 entity type][6 base36 random]
func NewID(entityType string) string {
	ts := uint64(time.Now().UnixMilli() - epochMs)
	rnd := rand.Uint32()

	tsPart := strconv.FormatUint(ts, 36)
	tsPart = strings.Repeat("0", max(0, 8-len(tsPart))) + tsPart

	rndPart := strconv.FormatUint(uint64(rnd)%2176782336, 36)
	rndPart = strings.Repeat("0", max(0, 6-len(rndPart))) + rndPart

	return tsPart + entityType + rndPart
}

// ParseID extracts entity type and timestamp from an OrbitID string.
func ParseID(id string) (OrbitID, error) {
	if len(id) != 16 {
		return OrbitID{}, fmt.Errorf("invalid orbit id length: %d", len(id))
	}
	tsPart := id[:8]
	entityType := id[8:10]
	tsVal, err := strconv.ParseUint(tsPart, 36, 64)
	if err != nil {
		return OrbitID{}, fmt.Errorf("invalid orbit id timestamp: %w", err)
	}
	ts := time.UnixMilli(int64(tsVal) + epochMs)
	return OrbitID{Raw: id, EntityType: entityType, Timestamp: ts}, nil
}

// IsID reports whether s matches the OrbitID format (16 chars, valid base36 timestamp).
func IsID(s string) bool {
	if len(s) != 16 {
		return false
	}
	_, err := ParseID(s)
	return err == nil
}
