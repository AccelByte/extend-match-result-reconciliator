// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"encoding/json"
)

// MatchResultRequest represents the request body for submitting match results
type MatchResultRequest struct {
	Players   []string               `json:"players"`
	Winner    string                 `json:"winner"`
	Stats     map[string]PlayerStats `json:"stats"`
	StartTime int64                  `json:"startTime"`
	EndTime   int64                  `json:"endTime"`
}

// PlayerStats represents individual player statistics
type PlayerStats struct {
	MMR int32 `json:"mmr"`
}

// MatchResultResponse represents the response for match result submission
type MatchResultResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// KafkaMatchResultMessage represents the Kafka message format (reuses MatchResultRequest)
type KafkaMatchResultMessage struct {
	Sender  string `json:"sender"`
	MatchID string `json:"matchID"`
	MatchResultRequest
	SubmittedAt string `json:"submittedAt"`
	Version     int    `json:"version"`
}

// KafkaMatchStats represents the match statistics for Kafka format (deprecated - kept for backward compatibility)
type KafkaMatchStats struct {
	Winner      string                 `json:"winner"`
	PlayerStats map[string]PlayerStats `json:",inline"`
}

// MarshalJSON custom marshal for KafkaMatchStats to flatten the structure (deprecated - kept for backward compatibility)
func (k KafkaMatchStats) MarshalJSON() ([]byte, error) {
	// Create a map to flatten the structure
	statsMap := make(map[string]interface{})
	statsMap["winner"] = k.Winner

	// Add each player's stats directly to the map
	for playerID, stats := range k.PlayerStats {
		statsMap[playerID] = map[string]interface{}{
			"mmr": stats.MMR,
		}
	}

	return json.Marshal(statsMap)
}
