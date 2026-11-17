package model

import (
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// channelCooldownEntry tracks recent failures of one channel for one model.
type channelCooldownEntry struct {
	failures []time.Time
	until    time.Time
}

var (
	channelCooldownMu sync.Mutex
	channelCooldowns  = make(map[string]*channelCooldownEntry)
	cooldownNow       = time.Now
)

// channelCooldownSweepSize bounds the tracker: once it holds this many
// entries, recording a failure first drops the entries that no longer matter.
const channelCooldownSweepSize = 10000

func channelCooldownKey(channelID int, modelName string) string {
	return strconv.Itoa(channelID) + "|" + modelName
}

// RecordChannelCooldownFailure counts one failure of the channel for the model.
// When the failures within the window reach the threshold, the channel stops
// receiving the model for the cooldown period and true is returned.
func RecordChannelCooldownFailure(channelID int, modelName string) bool {
	if !common.RetryCooldownEnabled || channelID <= 0 {
		return false
	}
	window := time.Duration(common.RetryCooldownWindowSeconds) * time.Second
	threshold := common.RetryCooldownFailureThreshold
	duration := time.Duration(common.RetryCooldownSeconds) * time.Second
	if window <= 0 || threshold <= 0 || duration <= 0 {
		return false
	}
	now := cooldownNow()

	channelCooldownMu.Lock()
	defer channelCooldownMu.Unlock()
	if len(channelCooldowns) >= channelCooldownSweepSize {
		for key, entry := range channelCooldowns {
			if !now.Before(entry.until) && (len(entry.failures) == 0 || now.Sub(entry.failures[len(entry.failures)-1]) > window) {
				delete(channelCooldowns, key)
			}
		}
	}
	key := channelCooldownKey(channelID, modelName)
	entry := channelCooldowns[key]
	if entry == nil {
		entry = &channelCooldownEntry{}
		channelCooldowns[key] = entry
	}
	if now.Before(entry.until) {
		return false
	}
	recent := entry.failures[:0]
	for _, failedAt := range entry.failures {
		if now.Sub(failedAt) <= window {
			recent = append(recent, failedAt)
		}
	}
	entry.failures = append(recent, now)
	if len(entry.failures) < threshold {
		return false
	}
	entry.failures = nil
	entry.until = now.Add(duration)
	return true
}

// IsChannelModelCooling reports whether the channel is in cooldown for the model.
func IsChannelModelCooling(channelID int, modelName string) bool {
	if !common.RetryCooldownEnabled {
		return false
	}
	channelCooldownMu.Lock()
	defer channelCooldownMu.Unlock()
	entry := channelCooldowns[channelCooldownKey(channelID, modelName)]
	return entry != nil && cooldownNow().Before(entry.until)
}

// ResetChannelCooldowns clears every failure count and cooldown.
func ResetChannelCooldowns() {
	channelCooldownMu.Lock()
	defer channelCooldownMu.Unlock()
	channelCooldowns = make(map[string]*channelCooldownEntry)
}

// withoutCoolingChannels drops the channels cooling down for the model. The
// input slice is never mutated.
func withoutCoolingChannels(ids []int, modelName string) []int {
	if !common.RetryCooldownEnabled || len(ids) == 0 {
		return ids
	}
	kept := make([]int, 0, len(ids))
	for _, id := range ids {
		if !IsChannelModelCooling(id, modelName) {
			kept = append(kept, id)
		}
	}
	return kept
}
