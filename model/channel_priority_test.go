package model

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestChannelPriorityPlan(t *testing.T) {
	for _, tc := range []struct {
		name       string
		values     []int64
		id, target int
		position   string
		want       map[int]int64
	}{
		{"midpoint", []int64{100, 80, 0}, 3, 2, "before", map[int]int64{3: 90}},
		{"top preserves peers", []int64{0, 0, 0}, 3, 1, "before", map[int]int64{3: 10}},
		{"bottom", []int64{0, 0, 0}, 1, 3, "after", map[int]int64{1: -10}},
		{"same tier", []int64{100, 80, 0}, 3, 2, "same", map[int]int64{3: 80}},
		{"same slot", []int64{100, 80, 0}, 2, 1, "after", map[int]int64{}},
		{"same tier no op", []int64{0, 0}, 1, 2, "same", map[int]int64{}},
		{"shift smaller upper tier", []int64{100, 99, 99, 0}, 4, 2, "before", map[int]int64{1: 101, 4: 100}},
		{"shift smaller lower tier", []int64{100, 100, 99, 0}, 4, 3, "before", map[int]int64{3: 98, 4: 99}},
		{"shift complete adjacent tiers", []int64{102, 101, 100, 99, 0}, 5, 3, "after", map[int]int64{4: 98, 5: 99}},
		{"upper safe bound", []int64{maxSortablePriority, 0}, 2, 1, "before", map[int]int64{1: maxSortablePriority - 1, 2: maxSortablePriority}},
		{"lower safe bound", []int64{-maxSortablePriority, 0}, 2, 1, "after", map[int]int64{1: -maxSortablePriority + 1, 2: -maxSortablePriority}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]ChannelPriority, len(tc.values))
			for i, value := range tc.values {
				items[i] = ChannelPriority{ID: i + 1, Priority: &value}
			}
			changes, err := planChannelPriorityMove(items, ChannelPriorityMove{ID: tc.id, TargetID: tc.target, Position: tc.position})
			require.NoError(t, err)
			got := map[int]int64{}
			for _, change := range changes {
				require.NotNil(t, change.Priority)
				got[change.ID] = *change.Priority
			}
			assert.Equal(t, tc.want, got)
		})
	}
	_, err := planChannelPriorityMove([]ChannelPriority{{ID: 1}, {ID: 2}}, ChannelPriorityMove{ID: 1, TargetID: 2, Position: "invalid"})
	assert.ErrorIs(t, err, ErrChannelPriorityInvalid)
}

func TestChannelPriorityDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "priority_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&Ability{}, &Channel{}))
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				initCol()
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			high, low, zero := int64(100), int64(99), int64(0)
			w1, w2, w3, w4 := uint(11), uint(22), uint(33), uint(44)
			channels := []Channel{
				{Name: "high", Priority: &high, Weight: &w1, Key: "preserved", Status: 1},
				{Name: "low", Priority: &low, Weight: &w2, Status: 1},
				{Name: "peer", Priority: &low, Weight: &w3, Status: 1},
				{Name: "moving", Priority: &zero, Weight: &w4, Status: 1},
			}
			require.NoError(t, db.Create(&channels).Error)
			for _, channel := range channels {
				for _, group := range []string{"default", "second"} {
					require.NoError(t, db.Create(&Ability{Group: group, Model: "test", ChannelId: channel.Id, Priority: channel.Priority, Weight: uint(*channel.Weight), Enabled: true}).Error)
				}
			}
			initial, err := GetChannelPriorityOrder()
			require.NoError(t, err)
			move := ChannelPriorityMove{Revision: initial.Revision, ID: channels[3].Id, TargetID: channels[1].Id, Position: "before", Preview: true}
			preview, err := ReorderChannelPriority(move)
			require.NoError(t, err)
			require.Len(t, preview.Changes, 2)
			unchanged, err := GetChannelPriorityOrder()
			require.NoError(t, err)
			assert.Equal(t, initial, unchanged, "preview does not persist")
			move.Preview = false
			applied, err := ReorderChannelPriority(move)
			require.NoError(t, err)
			assert.Equal(t, preview, applied)
			for _, change := range applied.Changes {
				var abilities []Ability
				require.NoError(t, db.Where("channel_id = ?", change.ID).Find(&abilities).Error)
				require.Len(t, abilities, 2)
				for _, ability := range abilities {
					assert.Equal(t, change.Priority, ability.Priority)
					assert.True(t, ability.Enabled)
				}
			}
			var saved Channel
			require.NoError(t, db.First(&saved, channels[0].Id).Error)
			assert.Equal(t, "preserved", saved.Key)
			assert.Equal(t, channels[0].Weight, saved.Weight)
			_, err = ReorderChannelPriority(move)
			assert.ErrorIs(t, err, ErrChannelPriorityConflict)
			undo := ChannelPriorityMove{Revision: applied.Revision}
			for _, change := range applied.Changes {
				undo.Restore = append(undo.Restore, ChannelPriorityChange{ID: change.ID, Previous: change.Priority, Priority: change.Previous})
			}
			restored, err := ReorderChannelPriority(undo)
			require.NoError(t, err)
			assert.Equal(t, initial.Items, restored.Items)
			assert.Equal(t, initial.Revision, restored.Revision)
			if dialect != "sqlite" {
				sqlDB.SetMaxOpenConns(2)
			}
			start := make(chan struct{})
			errorsCh := make(chan error, 2)
			var workers sync.WaitGroup
			for range 2 {
				workers.Go(func() { <-start; _, moveErr := ReorderChannelPriority(move); errorsCh <- moveErr })
			}
			close(start)
			workers.Wait()
			close(errorsCh)
			succeeded, conflicted := 0, 0
			for moveErr := range errorsCh {
				if moveErr == nil {
					succeeded++
				} else {
					require.ErrorIs(t, moveErr, ErrChannelPriorityConflict)
					conflicted++
				}
			}
			assert.Equal(t, 1, succeeded)
			assert.Equal(t, 1, conflicted)
			_, err = ReorderChannelPriority(undo)
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			// An ability write failure must roll back every channel change too.
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("priority_test_failure", func(tx *gorm.DB) {
				if tx.Statement.Table == "priority_test_abilities" {
					tx.AddError(errors.New("forced routing write failure"))
				}
			}))
			_, err = ReorderChannelPriority(move)
			require.ErrorContains(t, err, "forced routing write failure")
			require.NoError(t, db.Callback().Update().Remove("priority_test_failure"))
			unchanged, err = GetChannelPriorityOrder()
			require.NoError(t, err)
			assert.Equal(t, initial, unchanged)
			// Null priority is an effective zero and undo must restore the null itself.
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channels[3].Id).Update("priority", nil).Error)
			nullable, err := GetChannelPriorityOrder()
			require.NoError(t, err)
			var sorted []ChannelPriority
			require.NoError(t, NewChannelSortOptions("priority", "asc", false).Apply(db.Model(&Channel{}).Select("id", "priority")).Find(&sorted).Error)
			assert.Equal(t, []int{channels[3].Id, channels[2].Id, channels[1].Id, channels[0].Id}, []int{sorted[0].ID, sorted[1].ID, sorted[2].ID, sorted[3].ID})
			move.Revision = nullable.Revision
			applied, err = ReorderChannelPriority(move)
			require.NoError(t, err)
			undo = ChannelPriorityMove{Revision: applied.Revision}
			for _, change := range applied.Changes {
				undo.Restore = append(undo.Restore, ChannelPriorityChange{ID: change.ID, Previous: change.Priority, Priority: change.Previous})
			}
			restored, err = ReorderChannelPriority(undo)
			require.NoError(t, err)
			assert.Equal(t, nullable.Items, restored.Items)
		})
	}
}

func TestGroupChannelSchedulingAndCostCalculation(t *testing.T) {
	chOfficial := &Channel{
		Id:     101,
		Name:   "OpenAI Official",
		Type:   1, // OpenAI official -> defaults to 1x multiplier
		Models: "deepseek-v4",
		Status: common.ChannelStatusEnabled,
	}
	chLowCost := &Channel{
		Id:     102,
		Name:   "NewAPI Upstream Cheap",
		Type:   60, // NewAPI
		Models: "deepseek-v4",
		Status: common.ChannelStatusEnabled,
	}
	chFast := &Channel{
		Id:     103,
		Name:   "NewAPI Upstream Fast",
		Type:   60,
		Models: "deepseek-v4",
		Status: common.ChannelStatusEnabled,
	}

	// Official API defaults to 1x multiplier
	officialEntry := computeDefaultModelCostEntry(chOfficial, "deepseek-v4", 1000)
	assert.Equal(t, 1.0, officialEntry.BaseMultiplier)
	SetChannelUpstreamModelCosts(chOfficial.Id, map[string]ChannelModelCostEntry{
		"deepseek-v4": officialEntry,
	}, false)

	// NewAPI upstream with explicit 1x model price (input $0.1/1M, output $0.3/1M)
	SetChannelUpstreamModelCosts(chLowCost.Id, map[string]ChannelModelCostEntry{
		"deepseek-v4": {
			ChannelID:       chLowCost.Id,
			ChannelName:     chLowCost.Name,
			ChannelType:     chLowCost.Type,
			Model:           "deepseek-v4",
			InputCost:       0.1,
			OutputCost:      0.3,
			ModelRatio:      0.05, // $0.1 / 1M tokens ($2 * 0.05)
			CompletionRatio: 3.0,  // $0.3 / 1M tokens
			BaseMultiplier:  1.0,
			Currency:        "USD",
			ExchangeRate:    1.0,
			FromUpstream:    true,
		},
	}, false)
	cheapCost, ok := GetChannelModelCost(chLowCost.Id, "deepseek-v4")
	require.True(t, ok)
	assert.InDelta(t, 0.1, cheapCost.InputCost, 1e-6)
	assert.InDelta(t, 0.3, cheapCost.OutputCost, 1e-6)

	SetChannelUpstreamModelCosts(chFast.Id, map[string]ChannelModelCostEntry{
		"deepseek-v4": {
			ChannelID:       chFast.Id,
			ChannelName:     chFast.Name,
			ChannelType:     chFast.Type,
			Model:           "deepseek-v4",
			InputCost:       0.5,
			OutputCost:      1.5,
			ModelRatio:      0.25, // $0.5 / 1M tokens
			CompletionRatio: 3.0,  // $1.5 / 1M tokens
			BaseMultiplier:  1.0,
			Currency:        "USD",
			ExchangeRate:    1.0,
			FromUpstream:    true,
		},
	}, false)

	// Record telemetry: chLowCost has 100% availability, TPS min=60, avg=126.67, max=200, avg TTFT=2.0s
	RecordChannelModelTelemetry(chLowCost.Id, "deepseek-v4", true, 2500, true, 60, 1000)
	RecordChannelModelTelemetry(chLowCost.Id, "deepseek-v4", true, 1500, true, 120, 1000)
	RecordChannelModelTelemetry(chLowCost.Id, "deepseek-v4", true, 2000, true, 200, 1000)

	// chFast has higher TPS (300 TPS) and lower TTFT (400ms), with 1 failure after 1 success
	RecordChannelModelTelemetry(chFast.Id, "deepseek-v4", true, 400, true, 300, 1000)
	RecordChannelModelTelemetry(chFast.Id, "deepseek-v4", false, 500, true, 0, 1000)

	snap := GetChannelModelTelemetry(chLowCost.Id, "deepseek-v4")
	assert.InDelta(t, 100.0, snap.Availability, 0.01)
	assert.InDelta(t, 60.0, snap.TpsMin, 0.1)
	assert.InDelta(t, 200.0, snap.TpsMax, 0.1)
	assert.InDelta(t, 2.0, snap.AvgTtftSec, 0.1)

	candidates := []*Channel{chOfficial, chLowCost, chFast}

	// LowerCost scheduling picks chLowCost
	groupSchedulingMu.Lock()
	groupSchedulingModes["vip"] = SchedulingModeLowerCost
	groupSchedulingMu.Unlock()
	pickedCost := selectChannelByGroupSchedule("vip", "deepseek-v4", 0, candidates, nil)
	require.NotNil(t, pickedCost)
	assert.Equal(t, chLowCost.Id, pickedCost.Id)

	// When chLowCost was already tried on an earlier attempt, retry skips it and picks the next best
	pickedCostRetry := selectChannelByGroupSchedule("vip", "deepseek-v4", 1, candidates, []int{chLowCost.Id})
	require.NotNil(t, pickedCostRetry)
	assert.NotEqual(t, chLowCost.Id, pickedCostRetry.Id)

	// Speed scheduling picks chFast (highest TPS=300 & lowest TTFT=400ms)
	groupSchedulingMu.Lock()
	groupSchedulingModes["vip"] = SchedulingModeSpeed
	groupSchedulingMu.Unlock()
	pickedSpeed := selectChannelByGroupSchedule("vip", "deepseek-v4", 0, candidates, nil)
	require.NotNil(t, pickedSpeed)
	assert.Equal(t, chFast.Id, pickedSpeed.Id)

	// Availability scheduling picks chLowCost (100% availability vs 50% on chFast)
	groupSchedulingMu.Lock()
	groupSchedulingModes["vip"] = SchedulingModeAvailability
	groupSchedulingMu.Unlock()
	pickedAvail := selectChannelByGroupSchedule("vip", "deepseek-v4", 0, []*Channel{chLowCost, chFast}, nil)
	require.NotNil(t, pickedAvail)
	assert.Equal(t, chLowCost.Id, pickedAvail.Id)

	// Simulate severe degradation on chFast so BBR congestion control penalizes it
	for range 5 {
		RecordChannelModelTelemetry(chFast.Id, "deepseek-v4", false, 3000, true, 0, 1000)
	}
	groupSchedulingMu.Lock()
	groupSchedulingModes["vip"] = SchedulingModeBBR
	groupSchedulingMu.Unlock()
	pickedBBR := selectChannelByGroupSchedule("vip", "deepseek-v4", 0, []*Channel{chLowCost, chFast}, nil)
	require.NotNil(t, pickedBBR)
	assert.Equal(t, chLowCost.Id, pickedBBR.Id)

	// Verify EMA recovery over time so a degraded channel is not permanently dead
	recovered := recoverChannelSuccessRate(0.2, time.Now().Add(-15*time.Minute), time.Now())
	assert.Greater(t, recovered, 0.85)
}
