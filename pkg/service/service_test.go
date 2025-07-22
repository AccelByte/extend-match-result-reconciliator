// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"
	"extend-match-result-reconciliator/pkg/common"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCompareMatchResults(t *testing.T) {
	// Create a test service
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests

	config := &Config{
		Redis: RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			TTL:      300 * time.Second,
		},
	}

	service := &Service{
		logger:    logger,
		config:    config,
		namespace: "test",
	}

	ctx := context.Background()

	// Base match result for testing
	baseResult := &common.KafkaMatchResultMessage{
		Sender:  "player1",
		MatchID: "match123",
		MatchResultRequest: common.MatchResultRequest{
			Players:   []string{"player1", "player2"},
			Winner:    "player1",
			StartTime: 1640995200000, // 2022-01-01 00:00:00 UTC
			EndTime:   1640995500000, // 2022-01-01 00:05:00 UTC
			Stats: map[string]common.PlayerStats{
				"player1": {MMR: 1500},
				"player2": {MMR: 1400},
			},
		},
		SubmittedAt: "2022-01-01T00:05:10Z",
		Version:     1,
	}

	t.Run("Identical results should match", func(t *testing.T) {
		// Create identical results from both players
		result1 := *baseResult
		result1.Sender = "player1"

		result2 := *baseResult
		result2.Sender = "player2"

		comparison := service.compareMatchResults(ctx, &result1, &result2)

		assert.True(t, comparison.Match)
		assert.Empty(t, comparison.Discrepancies)
		assert.Equal(t, &result1, comparison.FirstResult)
		assert.Equal(t, &result2, comparison.SecondResult)
	})

	t.Run("Different winners should not match", func(t *testing.T) {
		result1 := *baseResult
		result1.Sender = "player1"
		result1.Winner = "player1"

		result2 := *baseResult
		result2.Sender = "player2"
		result2.Winner = "player2"

		comparison := service.compareMatchResults(ctx, &result1, &result2)

		assert.False(t, comparison.Match)
		assert.Contains(t, comparison.Discrepancies, "winner mismatch: player1 vs player2")
	})

	t.Run("Different player stats should not match", func(t *testing.T) {
		result1 := *baseResult
		result1.Sender = "player1"

		result2 := *baseResult
		result2.Sender = "player2"
		result2.Stats = map[string]common.PlayerStats{
			"player1": {MMR: 1500},
			"player2": {MMR: 1450}, // Different MMR
		}

		comparison := service.compareMatchResults(ctx, &result1, &result2)

		assert.False(t, comparison.Match)
		assert.Contains(t, comparison.Discrepancies, "player player2 MMR mismatch: 1400 vs 1450")
	})

	t.Run("Time differences within tolerance should match", func(t *testing.T) {
		result1 := *baseResult
		result1.Sender = "player1"
		result1.StartTime = 1640995200000
		result1.EndTime = 1640995500000

		result2 := *baseResult
		result2.Sender = "player2"
		result2.StartTime = 1640995202000 // 2 seconds later
		result2.EndTime = 1640995503000   // 3 seconds later

		comparison := service.compareMatchResults(ctx, &result1, &result2)

		assert.True(t, comparison.Match)
		assert.Empty(t, comparison.Discrepancies)
	})

	t.Run("Time differences beyond tolerance should not match", func(t *testing.T) {
		result1 := *baseResult
		result1.Sender = "player1"
		result1.StartTime = 1640995200000
		result1.EndTime = 1640995500000

		result2 := *baseResult
		result2.Sender = "player2"
		result2.StartTime = 1640995210000 // 10 seconds later (beyond 5s tolerance)
		result2.EndTime = 1640995510000   // 10 seconds later

		comparison := service.compareMatchResults(ctx, &result1, &result2)

		assert.False(t, comparison.Match)
		assert.Len(t, comparison.Discrepancies, 2) // Both start and end time mismatches
	})

	t.Run("Different player sets should not match", func(t *testing.T) {
		result1 := *baseResult
		result1.Sender = "player1"
		result1.Players = []string{"player1", "player2"}

		result2 := *baseResult
		result2.Sender = "player2"
		result2.Players = []string{"player1", "player3"} // Different second player

		comparison := service.compareMatchResults(ctx, &result1, &result2)

		assert.False(t, comparison.Match)
		assert.Contains(t, comparison.Discrepancies, "player player3 not found in first result")
	})

	t.Run("Missing player stats should not match", func(t *testing.T) {
		result1 := *baseResult
		result1.Sender = "player1"

		result2 := *baseResult
		result2.Sender = "player2"
		result2.Stats = map[string]common.PlayerStats{
			"player1": {MMR: 1500},
			// Missing player2 stats
		}

		comparison := service.compareMatchResults(ctx, &result1, &result2)

		assert.False(t, comparison.Match)
		assert.Contains(t, comparison.Discrepancies, "stats count mismatch: 2 vs 1")
	})
}

func TestValidateMessage(t *testing.T) {
	service := &Service{}

	t.Run("Valid message should pass validation", func(t *testing.T) {
		msg := &common.KafkaMatchResultMessage{
			Sender:  "player1",
			MatchID: "match123",
			MatchResultRequest: common.MatchResultRequest{
				Players:   []string{"player1", "player2"},
				Winner:    "player1",
				StartTime: 1640995200000,
				EndTime:   1640995500000,
				Stats: map[string]common.PlayerStats{
					"player1": {MMR: 1500},
					"player2": {MMR: 1400},
				},
			},
			Version: 1,
		}

		err := service.validateMessage(msg)
		assert.NoError(t, err)
	})

	t.Run("Message with empty players should fail", func(t *testing.T) {
		msg := &common.KafkaMatchResultMessage{
			Sender:  "player1",
			MatchID: "match123",
			MatchResultRequest: common.MatchResultRequest{
				Players:   []string{}, // Empty players list
				Winner:    "player1",
				StartTime: 1640995200000,
				EndTime:   1640995500000,
			},
			Version: 1,
		}

		err := service.validateMessage(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "players list cannot be empty")
	})

	t.Run("Message with sender not in players should fail", func(t *testing.T) {
		msg := &common.KafkaMatchResultMessage{
			Sender:  "player3", // Not in players list
			MatchID: "match123",
			MatchResultRequest: common.MatchResultRequest{
				Players:   []string{"player1", "player2"},
				Winner:    "player1",
				StartTime: 1640995200000,
				EndTime:   1640995500000,
			},
			Version: 1,
		}

		err := service.validateMessage(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sender must be one of the players")
	})

	t.Run("Message with winner not in players should fail", func(t *testing.T) {
		msg := &common.KafkaMatchResultMessage{
			Sender:  "player1",
			MatchID: "match123",
			MatchResultRequest: common.MatchResultRequest{
				Players:   []string{"player1", "player2"},
				Winner:    "player3", // Not in players list
				StartTime: 1640995200000,
				EndTime:   1640995500000,
			},
			Version: 1,
		}

		err := service.validateMessage(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "winner must be one of the players")
	})
}

func TestAbs(t *testing.T) {
	assert.Equal(t, int64(5), abs(5))
	assert.Equal(t, int64(5), abs(-5))
	assert.Equal(t, int64(0), abs(0))
}

func TestStoreMatchResultInCloudSave(t *testing.T) {
	// Create a test service
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests

	config := &Config{
		Redis: RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			TTL:      300 * time.Second,
		},
		AccelByte: AccelByteConfig{
			Enabled: false, // Disable for unit test
		},
	}

	service := &Service{
		logger:    logger,
		config:    config,
		namespace: "test",
	}

	ctx := context.Background()

	// Test match result
	matchResult := &common.KafkaMatchResultMessage{
		Sender:  "player1",
		MatchID: "match123",
		MatchResultRequest: common.MatchResultRequest{
			Players:   []string{"player1", "player2"},
			Winner:    "player1",
			StartTime: 1640995200000,
			EndTime:   1640995500000,
			Stats: map[string]common.PlayerStats{
				"player1": {MMR: 1500},
				"player2": {MMR: 1400},
			},
		},
		SubmittedAt: "2022-01-01T00:05:10Z",
		Version:     1,
	}

	t.Run("CloudSave disabled should not error", func(t *testing.T) {
		err := service.storeMatchResultInCloudSave(ctx, matchResult)
		assert.NoError(t, err)
	})

	t.Run("CloudSave services not initialized should error", func(t *testing.T) {
		// Enable CloudSave but don't initialize services
		service.config.AccelByte.Enabled = true
		err := service.storeMatchResultInCloudSave(ctx, matchResult)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CloudSave services not initialized")
	})
}
