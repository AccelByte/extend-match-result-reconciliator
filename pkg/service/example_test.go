// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"
	"extend-match-result-reconciliator/pkg/common"
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// ExampleService_compareMatchResults demonstrates how the reconciliator works with two players
func ExampleService_compareMatchResults() {
	// Create a logger for the example
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	config := &Config{
		Redis: RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			TTL:      300 * time.Second,
		},
	}

	// Create service (without Redis for this example)
	service := &Service{
		logger:    logger,
		config:    config,
		namespace: "example",
	}

	ctx := context.Background()

	// Player 1 submits match result
	player1Result := &common.KafkaMatchResultMessage{
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

	// Player 2 submits identical match result
	player2Result := &common.KafkaMatchResultMessage{
		Sender:  "player2",
		MatchID: "match123",
		MatchResultRequest: common.MatchResultRequest{
			Players:   []string{"player1", "player2"},
			Winner:    "player1",     // Same winner
			StartTime: 1640995202000, // 2 seconds later (within tolerance)
			EndTime:   1640995503000, // 3 seconds later (within tolerance)
			Stats: map[string]common.PlayerStats{
				"player1": {MMR: 1500},
				"player2": {MMR: 1400},
			},
		},
		SubmittedAt: "2022-01-01T00:05:13Z",
		Version:     1,
	}

	// Compare the results
	comparison := service.compareMatchResults(ctx, player1Result, player2Result)

	if comparison.Match {
		fmt.Printf("✅ Match results are consistent between players\n")
		fmt.Printf("   Match ID: %s\n", player1Result.MatchID)
		fmt.Printf("   Winner: %s\n", player1Result.Winner)
		fmt.Printf("   Players: %v\n", player1Result.Players)
	} else {
		fmt.Printf("❌ Match results are inconsistent!\n")
		fmt.Printf("   Discrepancies found:\n")
		for _, discrepancy := range comparison.Discrepancies {
			fmt.Printf("   - %s\n", discrepancy)
		}
	}

	// Output:
	// ✅ Match results are consistent between players
	//    Match ID: match123
	//    Winner: player1
	//    Players: [player1 player2]
}

// ExampleService_compareMatchResults_mismatch demonstrates what happens when players submit different results
func ExampleService_compareMatchResults_mismatch() {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

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
		namespace: "example",
	}

	ctx := context.Background()

	// Player 1 claims they won
	player1Result := &common.KafkaMatchResultMessage{
		Sender:  "player1",
		MatchID: "match456",
		MatchResultRequest: common.MatchResultRequest{
			Players:   []string{"player1", "player2"},
			Winner:    "player1", // Player 1 claims victory
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

	// Player 2 claims they won
	player2Result := &common.KafkaMatchResultMessage{
		Sender:  "player2",
		MatchID: "match456",
		MatchResultRequest: common.MatchResultRequest{
			Players:   []string{"player1", "player2"},
			Winner:    "player2", // Player 2 claims victory
			StartTime: 1640995202000,
			EndTime:   1640995503000,
			Stats: map[string]common.PlayerStats{
				"player1": {MMR: 1500},
				"player2": {MMR: 1450}, // Also different MMR
			},
		},
		SubmittedAt: "2022-01-01T00:05:13Z",
		Version:     1,
	}

	// Compare the results
	comparison := service.compareMatchResults(ctx, player1Result, player2Result)

	if comparison.Match {
		fmt.Printf("✅ Match results are consistent between players\n")
	} else {
		fmt.Printf("❌ Match results are inconsistent!\n")
		fmt.Printf("   Match ID: %s\n", player1Result.MatchID)
		fmt.Printf("   Discrepancies found:\n")
		for _, discrepancy := range comparison.Discrepancies {
			fmt.Printf("   - %s\n", discrepancy)
		}
	}

	// Output:
	// ❌ Match results are inconsistent!
	//    Match ID: match456
	//    Discrepancies found:
	//    - winner mismatch: player1 vs player2
	//    - player player2 MMR mismatch: 1400 vs 1450
}

func TestExamples(t *testing.T) {
	// Run the examples to ensure they work
	ExampleService_compareMatchResults()
	ExampleService_compareMatchResults_mismatch()
}
