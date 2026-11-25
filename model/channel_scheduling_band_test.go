package model

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSchedulingBandTestGroup(t *testing.T, group string, mode string, channelIDs ...int) {
	t.Helper()
	groupSchedulingMu.Lock()
	prevMode, hadMode := groupSchedulingModes[group]
	groupSchedulingModes[group] = mode
	groupSchedulingMu.Unlock()

	t.Cleanup(func() {
		groupSchedulingMu.Lock()
		if hadMode {
			groupSchedulingModes[group] = prevMode
		} else {
			delete(groupSchedulingModes, group)
		}
		groupSchedulingMu.Unlock()

		for _, id := range channelIDs {
			channelModelCostMu.Lock()
			delete(channelModelCostMap, id)
			channelModelCostMu.Unlock()
		}
		channelTelemetryMap.Range(func(k, _ any) bool {
			if key, ok := k.(channelModelKey); ok {
				for _, id := range channelIDs {
					if key.channelID == id {
						channelTelemetryMap.Delete(k)
						break
					}
				}
			}
			return true
		})
	})
}

func testWeightedChannel(id int, name string, weight uint) *Channel {
	w := weight
	return &Channel{
		Id:     id,
		Name:   name,
		Type:   60,
		Status: common.ChannelStatusEnabled,
		Weight: &w,
	}
}

func TestAvailabilityBandSharesTrafficAcrossHealthyChannels(t *testing.T) {
	const (
		group     = "sched-avail-share"
		modelName = "gpt-5-avail-share"
	)
	chA := testWeightedChannel(9101, "healthy-fast", 50)
	chB := testWeightedChannel(9102, "healthy-mid", 30)
	chC := testWeightedChannel(9103, "healthy-slow", 20)
	setupSchedulingBandTestGroup(t, group, SchedulingModeAvailability, chA.Id, chB.Id, chC.Id)

	// Distinct speeds and latencies, all 100% healthy (>= 0.99 band).
	for range 5 {
		RecordChannelModelTelemetry(chA.Id, modelName, true, 120, true, 1000, 1000)
		RecordChannelModelTelemetry(chB.Id, modelName, true, 400, true, 550, 1000)
		RecordChannelModelTelemetry(chC.Id, modelName, true, 900, true, 300, 1000)
	}

	assert.Equal(t, scoreChannelForSchedule(SchedulingModeAvailability, chA, modelName), scoreChannelForSchedule(SchedulingModeAvailability, chB, modelName))
	assert.Equal(t, scoreChannelForSchedule(SchedulingModeAvailability, chB, modelName), scoreChannelForSchedule(SchedulingModeAvailability, chC, modelName))

	hits := map[int]int{}
	candidates := []*Channel{chA, chB, chC}
	for range 200 {
		picked := selectChannelByGroupSchedule(group, modelName, 0, candidates, nil)
		require.NotNil(t, picked)
		hits[picked.Id]++
	}
	assert.Greater(t, hits[chA.Id], 0)
	assert.Greater(t, hits[chB.Id], 0)
	assert.Greater(t, hits[chC.Id], 0)
}

func TestAvailabilityRejectsDegradedChannel(t *testing.T) {
	const (
		group     = "sched-avail-reject"
		modelName = "gpt-5-avail-reject"
	)
	chHealthy := testWeightedChannel(9201, "healthy", 50)
	chDegraded := testWeightedChannel(9202, "degraded", 50)
	setupSchedulingBandTestGroup(t, group, SchedulingModeAvailability, chHealthy.Id, chDegraded.Id)

	for range 10 {
		RecordChannelModelTelemetry(chHealthy.Id, modelName, true, 500, true, 200, 1000)
		RecordChannelModelTelemetry(chDegraded.Id, modelName, false, 100, true, 0, 1000)
	}

	assert.Greater(t,
		scoreChannelForSchedule(SchedulingModeAvailability, chHealthy, modelName),
		scoreChannelForSchedule(SchedulingModeAvailability, chDegraded, modelName),
	)

	degradedHits := 0
	candidates := []*Channel{chHealthy, chDegraded}
	const samples = 2000
	for range samples {
		picked := selectChannelByGroupSchedule(group, modelName, 0, candidates, nil)
		require.NotNil(t, picked)
		if picked.Id == chDegraded.Id {
			degradedHits++
		}
	}
	assert.Less(t, float64(degradedHits)/float64(samples), 0.15)
}

func TestBBRDeliveryBandQuantisesThroughput(t *testing.T) {
	for _, tc := range []struct {
		ratio    float64
		expected float64
	}{
		{ratio: 1.00, expected: 1.0},
		{ratio: 0.92, expected: 1.0},
		{ratio: 0.80, expected: 0.80},
		{ratio: 0.55, expected: 0.60},
		{ratio: 0.30, expected: 0.40},
		{ratio: 0.10, expected: 0.20},
	} {
		assert.InDelta(t, tc.expected, schedulingBBRDeliveryBand(tc.ratio), 1e-9)
	}

	const (
		group     = "sched-bbr-band"
		modelName = "gpt-5-bbr-band"
	)
	chA := testWeightedChannel(9301, "bbr-1000tps", 50)
	chB := testWeightedChannel(9302, "bbr-550tps", 30)
	chC := testWeightedChannel(9303, "bbr-300tps", 20)
	setupSchedulingBandTestGroup(t, group, SchedulingModeBBR, chA.Id, chB.Id, chC.Id)

	for range 5 {
		RecordChannelModelTelemetry(chA.Id, modelName, true, 200, true, 1000, 1000)
		RecordChannelModelTelemetry(chB.Id, modelName, true, 300, true, 550, 1000)
		RecordChannelModelTelemetry(chC.Id, modelName, true, 400, true, 300, 1000)
	}

	scoreA := scoreChannelForSchedule(SchedulingModeBBR, chA, modelName)
	scoreB := scoreChannelForSchedule(SchedulingModeBBR, chB, modelName)
	scoreC := scoreChannelForSchedule(SchedulingModeBBR, chC, modelName)
	assert.InDelta(t, scoreA, scoreB, 1e-6)
	assert.InDelta(t, scoreB, scoreC, 1e-6)

	hits := map[int]int{}
	candidates := []*Channel{chA, chB, chC}
	for range 200 {
		picked := selectChannelByGroupSchedule(group, modelName, 0, candidates, nil)
		require.NotNil(t, picked)
		hits[picked.Id]++
	}
	assert.Greater(t, hits[chA.Id], 0)
	assert.Greater(t, hits[chB.Id], 0)
	assert.Greater(t, hits[chC.Id], 0)
}

func TestBBRDrainsFailedChannel(t *testing.T) {
	const (
		group     = "sched-bbr-drain"
		modelName = "gpt-5-bbr-drain"
	)
	chFailed := testWeightedChannel(9401, "bbr-failed", 50)
	chHealthy := testWeightedChannel(9402, "bbr-healthy", 50)
	setupSchedulingBandTestGroup(t, group, SchedulingModeBBR, chFailed.Id, chHealthy.Id)

	RecordChannelModelTelemetry(chFailed.Id, modelName, true, 150, true, 1000, 1000)
	for range 10 {
		RecordChannelModelTelemetry(chFailed.Id, modelName, false, 200, true, 0, 1000)
		RecordChannelModelTelemetry(chHealthy.Id, modelName, true, 300, true, 400, 1000)
	}

	degradedHits := 0
	candidates := []*Channel{chFailed, chHealthy}
	const samples = 2000
	for range samples {
		picked := selectChannelByGroupSchedule(group, modelName, 0, candidates, nil)
		require.NotNil(t, picked)
		if picked.Id == chFailed.Id {
			degradedHits++
		}
	}
	assert.Less(t, float64(degradedHits)/float64(samples), 0.15)
}

func TestProbeGivesDegradedChannelChanceToRecover(t *testing.T) {
	const (
		group     = "sched-probe-recover"
		modelName = "gpt-5-probe-recover"
	)
	chDegraded := testWeightedChannel(9501, "degraded", 50)
	chHealthy := testWeightedChannel(9502, "healthy", 50)
	setupSchedulingBandTestGroup(t, group, SchedulingModeBBR, chDegraded.Id, chHealthy.Id)

	for range 10 {
		RecordChannelModelTelemetry(chDegraded.Id, modelName, false, 200, true, 0, 1000)
		RecordChannelModelTelemetry(chHealthy.Id, modelName, true, 200, true, 500, 1000)
	}
	require.Less(t, GetChannelModelTelemetry(chDegraded.Id, modelName).EmaSuccessRate, schedulingDegradedBelow)

	probeHits := 0
	candidates := []*Channel{chDegraded, chHealthy}
	const samples = 4000
	for range samples {
		picked := selectChannelByGroupSchedule(group, modelName, 0, candidates, nil)
		require.NotNil(t, picked)
		if picked.Id == chDegraded.Id {
			probeHits++
		}
	}
	assert.Greater(t, probeHits, 0)
	assert.Less(t, float64(probeHits)/float64(samples), 0.15)

	// Single untried candidate after excluding already-tried channels still runs through the post-probe path cleanly.
	singlePicked := selectChannelByGroupSchedule(group, modelName, 1, candidates, []int{chHealthy.Id})
	require.NotNil(t, singlePicked)
	assert.Equal(t, chDegraded.Id, singlePicked.Id)
}

func TestLowerCostYieldsWhenUnreliable(t *testing.T) {
	const (
		group     = "sched-lowercost-yield"
		modelName = "gpt-5-lowercost-yield"
	)
	chCheapUnreliable := testWeightedChannel(9601, "cheap-flaky", 50)
	chExpensiveHealthy := testWeightedChannel(9602, "expensive-healthy", 50)
	setupSchedulingBandTestGroup(t, group, SchedulingModeLowerCost, chCheapUnreliable.Id, chExpensiveHealthy.Id)

	SetChannelUpstreamModelCosts(chCheapUnreliable.Id, map[string]ChannelModelCostEntry{
		modelName: {
			ChannelID:    chCheapUnreliable.Id,
			Model:        modelName,
			InputCost:    0.05,
			OutputCost:   0.15, // composite = 0.50 (100x cheaper than healthy)
			Currency:     "USD",
			ExchangeRate: 1.0,
			FromUpstream: true,
		},
	}, false)
	SetChannelUpstreamModelCosts(chExpensiveHealthy.Id, map[string]ChannelModelCostEntry{
		modelName: {
			ChannelID:    chExpensiveHealthy.Id,
			Model:        modelName,
			InputCost:    5.0,
			OutputCost:   15.0, // composite = 50.0
			Currency:     "USD",
			ExchangeRate: 1.0,
			FromUpstream: true,
		},
	}, false)

	// Drop cheap channel below 90% reliability gate (1 failure -> EMA = 0.80 >= 0.50).
	RecordChannelModelTelemetry(chCheapUnreliable.Id, modelName, true, 200, true, 200, 1000)
	RecordChannelModelTelemetry(chCheapUnreliable.Id, modelName, false, 200, true, 0, 1000)
	for range 5 {
		RecordChannelModelTelemetry(chExpensiveHealthy.Id, modelName, true, 200, true, 200, 1000)
	}

	cheapSnap := GetChannelModelTelemetry(chCheapUnreliable.Id, modelName)
	require.Less(t, cheapSnap.EmaSuccessRate, schedulingReliableBelow)
	require.GreaterOrEqual(t, cheapSnap.EmaSuccessRate, schedulingDegradedBelow)

	picked := selectChannelByGroupSchedule(group, modelName, 0, []*Channel{chCheapUnreliable, chExpensiveHealthy}, nil)
	require.NotNil(t, picked)
	assert.Equal(t, chExpensiveHealthy.Id, picked.Id)
}

func TestLowerCostStillPrefersCheapWhenBothHealthy(t *testing.T) {
	const (
		group     = "sched-lowercost-healthy"
		modelName = "gpt-5-lowercost-healthy"
	)
	chCheap := testWeightedChannel(9701, "cheap-healthy", 50)
	chExpensive := testWeightedChannel(9702, "expensive-healthy", 50)
	setupSchedulingBandTestGroup(t, group, SchedulingModeLowerCost, chCheap.Id, chExpensive.Id)

	SetChannelUpstreamModelCosts(chCheap.Id, map[string]ChannelModelCostEntry{
		modelName: {
			ChannelID:    chCheap.Id,
			Model:        modelName,
			InputCost:    0.1,
			OutputCost:   0.3,
			Currency:     "USD",
			ExchangeRate: 1.0,
			FromUpstream: true,
		},
	}, false)
	SetChannelUpstreamModelCosts(chExpensive.Id, map[string]ChannelModelCostEntry{
		modelName: {
			ChannelID:    chExpensive.Id,
			Model:        modelName,
			InputCost:    1.0,
			OutputCost:   3.0,
			Currency:     "USD",
			ExchangeRate: 1.0,
			FromUpstream: true,
		},
	}, false)

	for range 5 {
		RecordChannelModelTelemetry(chCheap.Id, modelName, true, 200, true, 200, 1000)
		RecordChannelModelTelemetry(chExpensive.Id, modelName, true, 200, true, 200, 1000)
	}

	picked := selectChannelByGroupSchedule(group, modelName, 0, []*Channel{chCheap, chExpensive}, nil)
	require.NotNil(t, picked)
	assert.Equal(t, chCheap.Id, picked.Id)
}

func TestRecoveryHalfLifeStillApplies(t *testing.T) {
	now := time.Now()
	assert.InDelta(t, 0.2, recoverChannelSuccessRate(0.2, now, now), 1e-9)
	afterOneHalfLife := recoverChannelSuccessRate(0.2, now.Add(-channelHealthRecoveryHalfLife), now)
	assert.InDelta(t, 0.6, afterOneHalfLife, 1e-6)
	afterFourHalfLives := recoverChannelSuccessRate(0.2, now.Add(-4*channelHealthRecoveryHalfLife), now)
	assert.InDelta(t, 0.95, afterFourHalfLives, 1e-6)
}

func TestSpeedModeRanksNonStreamingByThroughputAndLatency(t *testing.T) {
	t.Run("with completion tokens", func(t *testing.T) {
		const (
			group     = "sched-speed-nonstream-tps"
			modelName = "mock-gpt-4-tps"
		)
		chA := testWeightedChannel(9801, "healthy-a", 50)
		chB := testWeightedChannel(9802, "healthy-b", 30)
		chC := testWeightedChannel(9803, "healthy-c", 20)
		setupSchedulingBandTestGroup(t, group, SchedulingModeSpeed, chA.Id, chB.Id, chC.Id)

		// Non-streaming samples: hasTtft == false, generationMs = total attempt latency.
		for range 5 {
			RecordChannelModelTelemetry(chA.Id, modelName, true, 0, false, 90, 26)  // ~3461 tps, 26ms
			RecordChannelModelTelemetry(chB.Id, modelName, true, 0, false, 22, 40)  // 550 tps, 40ms
			RecordChannelModelTelemetry(chC.Id, modelName, true, 0, false, 10, 100) // 100 tps, 100ms
		}

		scoreA := scoreChannelForSchedule(SchedulingModeSpeed, chA, modelName)
		scoreB := scoreChannelForSchedule(SchedulingModeSpeed, chB, modelName)
		scoreC := scoreChannelForSchedule(SchedulingModeSpeed, chC, modelName)
		assert.Greater(t, scoreA, scoreB)
		assert.Greater(t, scoreB, scoreC)

		aWins := 0
		for range 200 {
			picked := selectChannelByGroupSchedule(group, modelName, 0, []*Channel{chC, chB, chA}, nil)
			require.NotNil(t, picked)
			if picked.Id == chA.Id {
				aWins++
			}
		}
		assert.Greater(t, aWins, 170)
	})

	t.Run("without completion tokens uses attempt latency", func(t *testing.T) {
		const (
			group     = "sched-speed-nonstream-latency"
			modelName = "mock-gpt-4-latency"
		)
		chA := testWeightedChannel(9811, "fast-26ms", 50)
		chB := testWeightedChannel(9812, "mid-40ms", 30)
		chC := testWeightedChannel(9813, "slow-100ms", 20)
		setupSchedulingBandTestGroup(t, group, SchedulingModeSpeed, chA.Id, chB.Id, chC.Id)

		for range 5 {
			RecordChannelModelTelemetry(chA.Id, modelName, true, 0, false, 0, 26)
			RecordChannelModelTelemetry(chB.Id, modelName, true, 0, false, 0, 40)
			RecordChannelModelTelemetry(chC.Id, modelName, true, 0, false, 0, 100)
		}

		scoreA := scoreChannelForSchedule(SchedulingModeSpeed, chA, modelName)
		scoreB := scoreChannelForSchedule(SchedulingModeSpeed, chB, modelName)
		scoreC := scoreChannelForSchedule(SchedulingModeSpeed, chC, modelName)
		assert.Greater(t, scoreA, scoreB)
		assert.Greater(t, scoreB, scoreC)

		aWins := 0
		for range 200 {
			picked := selectChannelByGroupSchedule(group, modelName, 0, []*Channel{chC, chB, chA}, nil)
			require.NotNil(t, picked)
			if picked.Id == chA.Id {
				aWins++
			}
		}
		assert.Greater(t, aWins, 170)
	})
}

func TestSpeedModeColdStartExploresUnsampledFastChannel(t *testing.T) {
	const (
		group     = "sched-speed-coldstart"
		modelName = "mock-gpt-4-coldstart"
	)
	chFast := testWeightedChannel(9821, "fast-unsampled", 50)
	chMid := testWeightedChannel(9822, "mid-unsampled", 30)
	chSlow := testWeightedChannel(9823, "slow-sampled", 20)
	setupSchedulingBandTestGroup(t, group, SchedulingModeSpeed, chFast.Id, chMid.Id, chSlow.Id)

	// Slow channel (100 tps > 80.0 baseline) already has a sample; fast/mid start cold.
	RecordChannelModelTelemetry(chSlow.Id, modelName, true, 0, false, 10, 100)

	candidates := []*Channel{chSlow, chMid, chFast}
	hits := map[int]int{}
	for range 200 {
		picked := selectChannelByGroupSchedule(group, modelName, 0, candidates, nil)
		require.NotNil(t, picked)
		hits[picked.Id]++
		switch picked.Id {
		case chFast.Id:
			RecordChannelModelTelemetry(chFast.Id, modelName, true, 0, false, 90, 26)
		case chMid.Id:
			RecordChannelModelTelemetry(chMid.Id, modelName, true, 0, false, 22, 40)
		case chSlow.Id:
			RecordChannelModelTelemetry(chSlow.Id, modelName, true, 0, false, 10, 100)
		}
	}

	assert.Greater(t, hits[chFast.Id], hits[chSlow.Id])
}

func TestSpeedModeRecoversDegradedFastChannelViaProbe(t *testing.T) {
	const (
		group     = "sched-speed-probe"
		modelName = "mock-gpt-4-speed-probe"
	)
	chFastDegraded := testWeightedChannel(9831, "fast-degraded", 50)
	chSlowHealthy := testWeightedChannel(9832, "slow-healthy", 20)
	setupSchedulingBandTestGroup(t, group, SchedulingModeSpeed, chFastDegraded.Id, chSlowHealthy.Id)

	RecordChannelModelTelemetry(chFastDegraded.Id, modelName, true, 0, false, 90, 26)
	for range 10 {
		RecordChannelModelTelemetry(chFastDegraded.Id, modelName, false, 0, false, 0, 26)
		RecordChannelModelTelemetry(chSlowHealthy.Id, modelName, true, 0, false, 10, 100)
	}
	require.Less(t, GetChannelModelTelemetry(chFastDegraded.Id, modelName).EmaSuccessRate, schedulingDegradedBelow)

	probeHits := 0
	candidates := []*Channel{chFastDegraded, chSlowHealthy}
	const samples = 4000
	for range samples {
		picked := selectChannelByGroupSchedule(group, modelName, 0, candidates, nil)
		require.NotNil(t, picked)
		if picked.Id == chFastDegraded.Id {
			probeHits++
		}
	}
	assert.Greater(t, probeHits, 0)
	assert.Less(t, float64(probeHits)/float64(samples), 0.15)
}

func TestSpeedModeUnfreezesHealthyButPreviouslySlowChannel(t *testing.T) {
	const (
		group     = "sched-speed-slow-deadlock"
		modelName = "mock-gpt-4-slow-deadlock"
	)
	chA := testWeightedChannel(9841, "healthy-a-poisoned", 50)
	chC := testWeightedChannel(9842, "healthy-c-steady", 20)
	setupSchedulingBandTestGroup(t, group, SchedulingModeSpeed, chA.Id, chC.Id)

	// Poison A with 100%-successful 28s stall samples (ema_success_rate = 1.0, ema_latency_ms ≈ 28s, ema_tps ≈ 0.5).
	for range 8 {
		RecordChannelModelTelemetry(chA.Id, modelName, true, 0, false, 14, 28231)
		RecordChannelModelTelemetry(chC.Id, modelName, true, 0, false, 13, 164)
	}
	snapBefore := GetChannelModelTelemetry(chA.Id, modelName)
	require.Equal(t, 1.0, snapBefore.EmaSuccessRate)
	require.Greater(t, snapBefore.EmaLatencyMs, 20000.0)
	require.Less(t, snapBefore.EmaTps, 1.0)

	// A's upstream is now fast again (26ms, ~3461 tps). Verify speed-degraded probe unfreezes A in closed loop.
	candidates := []*Channel{chC, chA}
	hits := map[int]int{}
	for range 800 {
		picked := selectChannelByGroupSchedule(group, modelName, 0, candidates, nil)
		require.NotNil(t, picked)
		hits[picked.Id]++
		if picked.Id == chA.Id {
			RecordChannelModelTelemetry(chA.Id, modelName, true, 0, false, 90, 26)
		} else {
			RecordChannelModelTelemetry(chC.Id, modelName, true, 0, false, 13, 164)
		}
	}

	assert.Greater(t, hits[chA.Id], hits[chC.Id])
	snapAfter := GetChannelModelTelemetry(chA.Id, modelName)
	assert.Less(t, snapAfter.EmaLatencyMs, 100.0)
	assert.Greater(t, snapAfter.EmaTps, 1000.0)
}

func TestSpeedModePenalisesHighStreamingTTFTAndIdleDelayDecays(t *testing.T) {
	const (
		group     = "sched-speed-stream-ttft"
		modelName = "mock-gpt-4-stream-ttft"
	)
	chHealthyC := testWeightedChannel(9851, "healthy-c", 20)
	chSlow3s := testWeightedChannel(9852, "slow-3s-ttft", 5)
	setupSchedulingBandTestGroup(t, group, SchedulingModeSpeed, chHealthyC.Id, chSlow3s.Id)

	// slow-3s has 3000ms TTFT followed by 250 tps burst (15 tokens in 60ms);
	// healthy-c has 50ms TTFT and 100 tps (15 tokens in 150ms).
	for range 5 {
		RecordChannelModelTelemetry(chHealthyC.Id, modelName, true, 50, true, 15, 150)
		RecordChannelModelTelemetry(chSlow3s.Id, modelName, true, 3000, true, 15, 60)
	}

	assert.Greater(t,
		scoreChannelForSchedule(SchedulingModeSpeed, chHealthyC, modelName),
		scoreChannelForSchedule(SchedulingModeSpeed, chSlow3s, modelName),
	)

	now := time.Now()
	assert.InDelta(t, 14615.5, recoverChannelDelayMs(28231.0, now.Add(-channelHealthRecoveryHalfLife), now), 1e-6)
	assert.InDelta(t, 26.0, recoverChannelDelayMs(26.0, now.Add(-channelHealthRecoveryHalfLife), now), 1e-6)
	assert.InDelta(t, 40.25, recoverChannelTps(0.50, now.Add(-channelHealthRecoveryHalfLife), now), 1e-6)
	assert.InDelta(t, 3500.0, recoverChannelTps(3500.0, now.Add(-channelHealthRecoveryHalfLife), now), 1e-6)
}

func TestStandardRoutingWeightParityBetweenCacheAndDB(t *testing.T) {
	prevDB := DB
	prevMemCache := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "weight-parity.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB = prevDB
		common.MemoryCacheEnabled = prevMemCache
		_ = sqlDB.Close()
	})
	DB = db
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	for _, tc := range []struct {
		name           string
		wA             uint
		wB             uint
		minAShare      float64
		maxAShare      float64
		expectExactAll bool
	}{
		{name: "skewed 99 to 1 without +10 distortion", wA: 99, wB: 1, minAShare: 0.95, maxAShare: 1.0},
		{name: "zero weight yields zero share when peer has positive weight", wA: 10, wB: 0, minAShare: 1.0, maxAShare: 1.0, expectExactAll: true},
		{name: "all zero weights share traffic evenly", wA: 0, wB: 0, minAShare: 0.35, maxAShare: 0.65},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Ability{}).Error)
			require.NoError(t, db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Channel{}).Error)

			prio := int64(0)
			wA, wB := tc.wA, tc.wB
			chA := &Channel{Name: "ch-a", Type: 1, Status: common.ChannelStatusEnabled, Group: "parity", Models: "gpt-parity", Priority: &prio, Weight: &wA}
			chB := &Channel{Name: "ch-b", Type: 1, Status: common.ChannelStatusEnabled, Group: "parity", Models: "gpt-parity", Priority: &prio, Weight: &wB}
			require.NoError(t, db.Create(chA).Error)
			require.NoError(t, db.Create(chB).Error)
			require.NoError(t, chA.AddAbilities(db))
			require.NoError(t, chB.AddAbilities(db))

			common.MemoryCacheEnabled = true
			InitChannelCache()

			const samples = 1000
			cacheAHits := 0
			for range samples {
				picked, pickErr := GetRandomSatisfiedChannel("parity", "gpt-parity", 0, nil)
				require.NoError(t, pickErr)
				require.NotNil(t, picked)
				if picked.Id == chA.Id {
					cacheAHits++
				}
			}

			common.MemoryCacheEnabled = false
			dbAHits := 0
			for range samples {
				picked, pickErr := GetRandomSatisfiedChannel("parity", "gpt-parity", 0, nil)
				require.NoError(t, pickErr)
				require.NotNil(t, picked)
				if picked.Id == chA.Id {
					dbAHits++
				}
			}

			cacheShare := float64(cacheAHits) / float64(samples)
			dbShare := float64(dbAHits) / float64(samples)
			if tc.expectExactAll {
				assert.Equal(t, samples, cacheAHits)
				assert.Equal(t, samples, dbAHits)
			} else {
				assert.GreaterOrEqual(t, cacheShare, tc.minAShare)
				assert.LessOrEqual(t, cacheShare, tc.maxAShare)
				assert.GreaterOrEqual(t, dbShare, tc.minAShare)
				assert.LessOrEqual(t, dbShare, tc.maxAShare)
				assert.InDelta(t, cacheShare, dbShare, 0.08)
			}
		})
	}
}
