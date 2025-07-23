// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"extend-match-result-reconciliator/pkg/common"
	"extend-match-result-reconciliator/pkg/pb"
	"fmt"
	"strings"
	"time"

	"github.com/AccelByte/accelbyte-go-sdk/cloudsave-sdk/pkg/cloudsaveclient/admin_player_record"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/factory"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/repository"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/cloudsave"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/iam"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/session"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/social"
	sdkAuth "github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/utils/auth"
	"github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclient/game_session"
	"github.com/AccelByte/accelbyte-go-sdk/social-sdk/pkg/socialclient/user_statistic"
	"github.com/AccelByte/accelbyte-go-sdk/social-sdk/pkg/socialclientmodels"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Use shared message types from common package

var (
	// Custom Prometheus metrics for reconciliator
	kafkaMessagesProcessed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "reconciliator_kafka_messages_processed_total",
		Help: "Total number of Kafka messages processed",
	})

	kafkaMessageProcessingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "reconciliator_kafka_message_processing_duration_seconds",
		Help:    "Duration of Kafka message processing",
		Buckets: prometheus.DefBuckets,
	})

	redisOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliator_redis_operations_total",
		Help: "Total number of Redis operations",
	}, []string{"operation"})

	redisOperationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "reconciliator_redis_operation_duration_seconds",
		Help:    "Duration of Redis operations",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	reconciliatorErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliator_errors_total",
		Help: "Total number of reconciliator errors",
	}, []string{"type"})

	// New metrics for match comparison
	matchComparisonsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliator_match_comparisons_total",
		Help: "Total number of match comparisons performed",
	}, []string{"result"}) // result: "match", "mismatch", "error"

	matchComparisonDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "reconciliator_match_comparison_duration_seconds",
		Help:    "Duration of match comparison operations",
		Buckets: prometheus.DefBuckets,
	})

	matchResultsStored = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliator_match_results_stored_total",
		Help: "Total number of match results stored per match",
	}, []string{"count"}) // count: "first", "second"

	// New metrics for session validation
	sessionValidationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliator_session_validations_total",
		Help: "Total number of session validations performed",
	}, []string{"result"}) // result: "valid", "invalid", "error", "disabled"

	sessionValidationDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "reconciliator_session_validation_duration_seconds",
		Help:    "Duration of session validation operations",
		Buckets: prometheus.DefBuckets,
	})

	sessionValidationErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliator_session_validation_errors_total",
		Help: "Total number of session validation errors",
	}, []string{"type"}) // type: "api_error", "not_found", "invalid_members"

	// New metrics for CloudSave operations
	cloudsaveOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliator_cloudsave_operations_total",
		Help: "Total number of CloudSave operations",
	}, []string{"operation", "result"}) // operation: "get", "post"; result: "success", "error"

	cloudsaveOperationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "reconciliator_cloudsave_operation_duration_seconds",
		Help:    "Duration of CloudSave operations",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"}) // operation: "get", "post"

	matchResultsStoredToCloudsave = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "reconciliator_match_results_stored_to_cloudsave_total",
		Help: "Total number of match results stored to CloudSave per player",
	}, []string{"player_id"})
)

// Service implements the reconciliator service
type Service struct {
	pb.UnimplementedServiceServer
	logger      *logrus.Logger
	kafkaReader *kafka.Reader
	redisClient *redis.Client
	ctx         context.Context
	cancel      context.CancelFunc
	metrics     *prometheus.Registry
	config      *Config
	namespace   string

	// AccelByte SDK services
	oauthService   *iam.OAuth20Service
	sessionService *session.GameSessionService

	// CloudSave services
	adminPlayerRecordService *cloudsave.AdminPlayerRecordService
	userStatisticService     *social.UserStatisticService
}

// NewService creates a new reconciliator service instance
func NewService(logger *logrus.Logger, config *Config) *Service {
	if logger == nil {
		logger = logrus.New()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create metrics registry
	metrics := prometheus.NewRegistry()

	// Register custom metrics
	metrics.MustRegister(
		kafkaMessagesProcessed,
		kafkaMessageProcessingDuration,
		redisOperationsTotal,
		redisOperationDuration,
		reconciliatorErrors,
		matchComparisonsTotal,
		matchComparisonDuration,
		matchResultsStored,
		sessionValidationsTotal,
		sessionValidationDuration,
		sessionValidationErrors,
		cloudsaveOperationsTotal,
		cloudsaveOperationDuration,
		matchResultsStoredToCloudsave,
	)

	service := &Service{
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
		metrics:   metrics,
		config:    config,
		namespace: common.GetNamespace(),
	}

	// Initialize Kafka reader
	if kafkaReader := service.initKafkaReader(); kafkaReader != nil {
		service.kafkaReader = kafkaReader
		logger.Info("Kafka reader initialized successfully")
	} else {
		logger.Fatal("Failed to initialize Kafka reader")
	}

	// Initialize Redis client
	if redisClient := service.initRedisClient(); redisClient != nil {
		service.redisClient = redisClient
		logger.Info("Redis client initialized successfully")
	} else {
		logger.Fatal("Failed to initialize Redis client")
	}

	// Initialize AccelByte SDK services
	if err := service.initAccelByteSDK(); err != nil {
		logger.WithError(err).Fatal("Failed to initialize AccelByte SDK")
	}
	logger.Info("AccelByte SDK initialized successfully")

	return service
}

// initKafkaReader initializes the Kafka reader based on configuration
func (s *Service) initKafkaReader() *kafka.Reader {
	// Get Kafka configuration from config
	brokers := s.config.Kafka.Brokers
	topic := s.config.Kafka.Topic
	groupID := s.config.Kafka.GroupID
	username := s.config.Kafka.Username
	password := s.config.Kafka.Password

	if len(brokers) == 0 {
		s.logger.Error("Kafka brokers not configured")
		return nil
	}

	dialer := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
	}

	if username != "" && password != "" {
		mechanism, err := scram.Mechanism(scram.SHA512, username, password)
		if err != nil {
			s.logger.WithError(err).Fatal("Failed to create SASL SCRAM mechanism")
		}
		dialer.SASLMechanism = mechanism
		dialer.TLS = &tls.Config{InsecureSkipVerify: true}
		s.logger.Info("Kafka reader configured with SASL authentication")
	} else {
		s.logger.Info("Kafka reader configured without authentication")
	}

	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:          brokers,
		Topic:            topic,
		GroupID:          groupID,
		MinBytes:         s.config.Kafka.MinBytes,
		MaxBytes:         s.config.Kafka.MaxBytes,
		MaxWait:          time.Duration(s.config.Kafka.MaxWait) * time.Second,
		RebalanceTimeout: 30 * time.Second,
		SessionTimeout:   30 * time.Second,
		CommitInterval:   100 * time.Millisecond,
		ReadBatchTimeout: 5 * time.Second,
		StartOffset:      kafka.LastOffset,
		Dialer:           dialer,
	})
}

// initRedisClient initializes the Redis client based on configuration
func (s *Service) initRedisClient() *redis.Client {
	// Get Redis configuration from config
	redisAddr := s.config.Redis.Addr
	redisPassword := s.config.Redis.Password
	redisDB := s.config.Redis.DB

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		s.logger.WithError(err).Error("Failed to connect to Redis")
		return nil
	}

	return rdb
}

// initAccelByteSDK initializes the AccelByte SDK services
func (s *Service) initAccelByteSDK() error {
	// Create config repository (reads from environment variables)
	var configRepo repository.ConfigRepository = sdkAuth.DefaultConfigRepositoryImpl()

	// Create token repository
	var tokenRepo repository.TokenRepository = sdkAuth.DefaultTokenRepositoryImpl()

	// Initialize OAuth service
	s.oauthService = &iam.OAuth20Service{
		Client:           factory.NewIamClient(configRepo),
		ConfigRepository: configRepo,
		TokenRepository:  tokenRepo,
	}

	// Login using client credentials
	clientId := configRepo.GetClientId()
	clientSecret := configRepo.GetClientSecret()
	if err := s.oauthService.LoginClient(&clientId, &clientSecret); err != nil {
		return fmt.Errorf("failed to login with AccelByte client credentials: %w", err)
	}

	// Initialize Session service
	s.sessionService = &session.GameSessionService{
		Client:          factory.NewSessionClient(configRepo),
		TokenRepository: tokenRepo,
	}

	// Initialize CloudSave services
	s.adminPlayerRecordService = &cloudsave.AdminPlayerRecordService{
		Client:          factory.NewCloudsaveClient(configRepo),
		TokenRepository: tokenRepo,
	}

	// Initialize UserStatistic service
	s.userStatisticService = &social.UserStatisticService{
		Client:          factory.NewSocialClient(configRepo),
		TokenRepository: tokenRepo,
	}

	return nil
}

// GetMetricsRegistry returns the service's metrics registry
func (s *Service) GetMetricsRegistry() *prometheus.Registry {
	return s.metrics
}

// extractTraceContext extracts trace context from Kafka headers
func (s *Service) extractTraceContext(ctx context.Context, headers []kafka.Header) context.Context {
	return common.ExtractTraceContextFromKafkaHeaders(ctx, headers)
}

// Start begins consuming messages from Kafka
func (s *Service) Start() error {
	s.logger.Info("Starting reconciliator service...")

	// Start message consumption loop
	go s.consumeMessages()

	s.logger.Info("Reconciliator service started successfully")
	return nil
}

// consumeMessages runs the main message consumption loop
func (s *Service) consumeMessages() {
	tracer := otel.Tracer("reconciliator")

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Stopping message consumption...")
			return
		default:
			// Fetch message from Kafka (without auto-commit)
			msg, err := s.kafkaReader.FetchMessage(s.ctx)
			if err != nil {
				if err == context.Canceled {
					s.logger.Info("Kafka reader context canceled")
					return
				}
				s.logger.WithError(err).Error("Failed to fetch message from Kafka")
				continue
			}

			// Extract trace context from Kafka headers
			ctx := s.extractTraceContext(s.ctx, msg.Headers)

			// Process the message with tracing
			ctx, span := tracer.Start(ctx, "process_kafka_message",
				trace.WithAttributes(
					attribute.String("kafka.topic", msg.Topic),
					attribute.Int("kafka.partition", msg.Partition),
					attribute.Int64("kafka.offset", msg.Offset),
					attribute.String("kafka.key", string(msg.Key)),
				),
			)

			// Process the message and only commit if successful
			processErr := s.processMessage(ctx, msg)
			if processErr != nil {
				span.RecordError(processErr)
				s.logger.WithError(processErr).WithField("offset", msg.Offset).Error("Failed to process message")
				// Don't commit on error - message will be reprocessed
			} else {
				// Commit the message only after successful processing
				if commitErr := s.kafkaReader.CommitMessages(s.ctx, msg); commitErr != nil {
					s.logger.WithError(commitErr).WithField("offset", msg.Offset).Error("Failed to commit message")
					// Note: Message was processed successfully but commit failed
					// This could lead to reprocessing, so ensure your processing is idempotent
				} else {
					s.logger.WithFields(logrus.Fields{
						"topic":     msg.Topic,
						"partition": msg.Partition,
						"offset":    msg.Offset,
						"key":       string(msg.Key),
					}).Debug("Message processed and committed successfully")
				}
			}

			span.End()
		}
	}
}

// processMessage processes a single Kafka message
func (s *Service) processMessage(ctx context.Context, msg kafka.Message) error {
	tracer := otel.Tracer("reconciliator")

	// Record processing start time for metrics
	startTime := time.Now()
	defer func() {
		// Record processing duration
		duration := time.Since(startTime).Seconds()
		kafkaMessageProcessingDuration.Observe(duration)
		kafkaMessagesProcessed.Inc()
	}()

	// Parse the message with tracing
	ctx, parseSpan := tracer.Start(ctx, "parse_kafka_message")
	var matchResult common.KafkaMatchResultMessage
	if err := json.Unmarshal(msg.Value, &matchResult); err != nil {
		parseSpan.RecordError(err)
		parseSpan.End()
		reconciliatorErrors.WithLabelValues("parse_error").Inc()
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}
	parseSpan.SetAttributes(
		attribute.String("match.id", matchResult.MatchID),
		attribute.String("match.sender", matchResult.Sender),
		attribute.Int("match.players_count", len(matchResult.Players)),
	)
	parseSpan.End()

	// Validate the message with tracing
	ctx, validateSpan := tracer.Start(ctx, "validate_kafka_message")
	if err := s.validateMessage(&matchResult); err != nil {
		validateSpan.RecordError(err)
		validateSpan.End()
		reconciliatorErrors.WithLabelValues("validation_error").Inc()
		return fmt.Errorf("message validation failed: %w", err)
	}
	validateSpan.End()

	// Check if there's already a match result stored
	ctx, storeSpan := tracer.Start(ctx, "process_match_result",
		trace.WithAttributes(
			attribute.String("match.id", matchResult.MatchID),
			attribute.String("match.sender", matchResult.Sender),
			attribute.Int("match.players_count", len(matchResult.Players)),
			attribute.Int("match.version", matchResult.Version),
		),
	)
	defer storeSpan.End()

	// Check if there's already a stored result for this match
	existingResult, err := s.getStoredMatchResult(ctx, matchResult.MatchID)
	if err != nil {
		storeSpan.RecordError(err)
		reconciliatorErrors.WithLabelValues("redis_error").Inc()
		return fmt.Errorf("failed to get stored match result: %w", err)
	}

	if existingResult == nil {
		// This is the first result - store it
		if err := s.storeFirstMatchResult(ctx, &matchResult); err != nil {
			storeSpan.RecordError(err)
			reconciliatorErrors.WithLabelValues("redis_error").Inc()
			return fmt.Errorf("failed to store first match result: %w", err)
		}

		matchResultsStored.WithLabelValues("first").Inc()
		s.logger.WithFields(logrus.Fields{
			"sender":   matchResult.Sender,
			"match_id": matchResult.MatchID,
			"players":  matchResult.Players,
			"version":  matchResult.Version,
		}).Info("First match result stored in Redis")
		return nil
	}

	// This is the second result - check for duplicate sender first
	if existingResult.Sender == matchResult.Sender {
		s.logger.WithFields(logrus.Fields{
			"sender":   matchResult.Sender,
			"match_id": matchResult.MatchID,
		}).Warn("Duplicate submission detected, ignoring")
		return nil
	}

	// Compare with the existing result without storing
	ctx, compareSpan := tracer.Start(ctx, "compare_match_results_flow",
		trace.WithAttributes(
			attribute.String("match.id", matchResult.MatchID),
			attribute.String("first.sender", existingResult.Sender),
			attribute.String("second.sender", matchResult.Sender),
		),
	)
	defer compareSpan.End()

	comparisonResult := s.compareMatchResults(ctx, existingResult, &matchResult)

	matchResultsStored.WithLabelValues("second").Inc()

	if comparisonResult.Match {
		s.logger.WithFields(logrus.Fields{
			"match_id":      matchResult.MatchID,
			"players":       matchResult.Players,
			"first_sender":  existingResult.Sender,
			"second_sender": matchResult.Sender,
		}).Info("✅ Match results from both players are consistent")

		// Store match result in CloudSave for all players
		if err := s.storeMatchResultInCloudSave(ctx, &matchResult); err != nil {
			s.logger.WithError(err).WithFields(logrus.Fields{
				"match_id": matchResult.MatchID,
				"players":  matchResult.Players,
			}).Error("Failed to store match result in CloudSave")
			return fmt.Errorf("failed to store match result in CloudSave: %w", err)
		}

		// Update player statistics
		if err := s.updatePlayerStatistics(ctx, &matchResult); err != nil {
			s.logger.WithError(err).WithField("match_id", matchResult.MatchID).Error("Failed to update player statistics")
			return fmt.Errorf("failed to update player statistics: %w", err)
		}

		return nil
	}

	s.logger.WithFields(logrus.Fields{
		"match_id":      matchResult.MatchID,
		"players":       matchResult.Players,
		"first_sender":  existingResult.Sender,
		"second_sender": matchResult.Sender,
		"discrepancies": comparisonResult.Discrepancies,
	}).Warn("❌ Match results from players are inconsistent")

	// Validate session members only when there are player-related discrepancies
	// that could be resolved by checking session data
	playerRelatedFlags := DiscrepancyPlayersCount | DiscrepancyPlayersMismatch
	if comparisonResult.BitFlags&playerRelatedFlags != 0 {
		correctedMatchResult, err := s.validateSessionMembers(ctx, &matchResult)
		if err != nil {
			if strings.Contains(err.Error(), "game session not found") {
				// Do not trust any of the match results if there is no session
				s.logger.WithFields(logrus.Fields{
					"match_id":      matchResult.MatchID,
					"players":       matchResult.Players,
					"first_sender":  existingResult.Sender,
					"second_sender": matchResult.Sender,
				}).Info("Session validation failed for inconsistent match results - no session found - skipping")
				return nil
			}

			s.logger.WithError(err).WithFields(logrus.Fields{
				"match_id":      matchResult.MatchID,
				"players":       matchResult.Players,
				"first_sender":  existingResult.Sender,
				"second_sender": matchResult.Sender,
				"bit_flags":     comparisonResult.BitFlags,
			}).Error("Session validation failed for inconsistent match results")

			return fmt.Errorf("session validation failed for inconsistent match results: %w", err)
		}

		if correctedMatchResult != nil {
			// Update the match result with corrected data
			matchResult = *correctedMatchResult
		}
		s.logger.WithFields(logrus.Fields{
			"match_id":      matchResult.MatchID,
			"players":       matchResult.Players,
			"first_sender":  existingResult.Sender,
			"second_sender": matchResult.Sender,
			"bit_flags":     comparisonResult.BitFlags,
		}).Info("Session validation passed for inconsistent match results - corrected match result")
	} else {

		// TODO: Add more sophisticated handling for discrepancies
		// For now, just log the differences
		// Possible other discrepancies:
		// - Winner mismatch
		// - Start time mismatch
		// - End time mismatch
		// - Stats count mismatch
		// - Player stats mismatch

		s.logger.WithFields(logrus.Fields{
			"match_id":      matchResult.MatchID,
			"players":       matchResult.Players,
			"first_sender":  existingResult.Sender,
			"second_sender": matchResult.Sender,
			"bit_flags":     comparisonResult.BitFlags,
		}).Info("Skipping session validation - no player-related discrepancies detected")
	}

	// Store match result in CloudSave for all players
	if err := s.storeMatchResultInCloudSave(ctx, &matchResult); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"match_id": matchResult.MatchID,
			"players":  matchResult.Players,
		}).Error("Failed to store match result in CloudSave")
		return fmt.Errorf("failed to store match result in CloudSave: %w", err)
	}

	// Update player statistics
	if err := s.updatePlayerStatistics(ctx, &matchResult); err != nil {
		s.logger.WithError(err).WithField("match_id", matchResult.MatchID).Error("Failed to update player statistics")
		return fmt.Errorf("failed to update player statistics: %w", err)
	}

	return nil
}

// validateMessage validates the Kafka message structure
func (s *Service) validateMessage(msg *common.KafkaMatchResultMessage) error {
	if msg.Sender == "" {
		return fmt.Errorf("sender is required")
	}
	if msg.MatchID == "" {
		return fmt.Errorf("matchID is required")
	}
	if len(msg.Players) == 0 {
		return fmt.Errorf("players list cannot be empty")
	}
	if msg.Winner == "" {
		return fmt.Errorf("winner is required")
	}
	if msg.StartTime == 0 || msg.EndTime == 0 {
		return fmt.Errorf("startTime and endTime are required")
	}
	if msg.Version == 0 {
		return fmt.Errorf("version is required")
	}

	// Validate that winner is one of the players
	winnerFound := false
	for _, player := range msg.Players {
		if player == msg.Winner {
			winnerFound = true
			break
		}
	}
	if !winnerFound {
		return fmt.Errorf("winner must be one of the players in the match")
	}

	// Validate that sender is one of the players
	senderFound := false
	for _, player := range msg.Players {
		if player == msg.Sender {
			senderFound = true
			break
		}
	}
	if !senderFound {
		return fmt.Errorf("sender must be one of the players in the match")
	}

	return nil
}

// validateSessionMembers validates that the players in the match result are valid session members
// and overrides member data with session service data when discrepancies are found.
// Only returns errors for API failures, not validation failures.
func (s *Service) validateSessionMembers(ctx context.Context, matchResult *common.KafkaMatchResultMessage) (*common.KafkaMatchResultMessage, error) {
	tracer := otel.Tracer("reconciliator")

	// Record validation start time
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		sessionValidationDuration.Observe(duration)
	}()

	// Check if we have the required SDK services
	if s.sessionService == nil {
		sessionValidationErrors.WithLabelValues("not_initialized").Inc()
		return nil, fmt.Errorf("session service not initialized")
	}

	ctx, span := tracer.Start(ctx, "validate_session_members",
		trace.WithAttributes(
			attribute.String("match.id", matchResult.MatchID),
			attribute.String("match.sender", matchResult.Sender),
			attribute.Int("match.players_count", len(matchResult.Players)),
		),
	)
	defer span.End()

	// For now, we'll assume the match ID corresponds to a session ID
	// In a real implementation, you might need to map match IDs to session IDs
	sessionID := matchResult.MatchID

	// Prepare the GetGameSession request
	input := &game_session.GetGameSessionParams{
		Namespace: s.config.AccelByte.Namespace,
		SessionID: sessionID,
	}

	// Call the AccelByte Session API
	_, apiSpan := tracer.Start(ctx, "accelbyte_get_game_session",
		trace.WithAttributes(
			attribute.String("session.id", sessionID),
			attribute.String("session.namespace", s.config.AccelByte.Namespace),
		),
	)

	gameSession, err := s.sessionService.GetGameSessionShort(input)
	apiSpan.End()

	if err != nil {
		if errors.Is(err, game_session.NewGetGameSessionNotFound()) {
			sessionValidationErrors.WithLabelValues("not_found").Inc()
			sessionValidationsTotal.WithLabelValues("error").Inc()
			return nil, fmt.Errorf("game session not found: %s", sessionID)
		}
		sessionValidationErrors.WithLabelValues("api_error").Inc()
		sessionValidationsTotal.WithLabelValues("error").Inc()
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get game session: %w", err)
	}

	if gameSession == nil {
		sessionValidationErrors.WithLabelValues("not_found").Inc()
		sessionValidationsTotal.WithLabelValues("error").Inc()
		return nil, fmt.Errorf("game session not found: %s", sessionID)
	}

	// Extract session members from the response
	var sessionMembersList []string
	sessionMembers := make(map[string]bool)
	if gameSession.Members != nil {
		for _, member := range gameSession.Members {
			if member.ID != nil {
				sessionMembersList = append(sessionMembersList, *member.ID)
				sessionMembers[*member.ID] = true
			}
		}
	}

	// Create a copy of the match result to potentially modify
	correctedMatchResult := *matchResult

	// Check for discrepancies and override with session data
	var invalidPlayers []string
	for _, playerID := range matchResult.Players {
		if !sessionMembers[playerID] {
			invalidPlayers = append(invalidPlayers, playerID)
		}
	}

	if len(invalidPlayers) > 0 {
		sessionValidationErrors.WithLabelValues("invalid_members").Inc()
		sessionValidationsTotal.WithLabelValues("corrected").Inc()

		// Override players list with session members
		correctedMatchResult.Players = sessionMembersList

		// If the winner is not in the session members, clear the winner
		if !sessionMembers[matchResult.Winner] {
			correctedMatchResult.Winner = "" // Will need to be determined by game logic
		}

		span.SetAttributes(
			attribute.StringSlice("invalid.players", invalidPlayers),
			attribute.Int("invalid.players_count", len(invalidPlayers)),
			attribute.StringSlice("corrected.players", sessionMembersList),
			attribute.Bool("corrected.winner", correctedMatchResult.Winner != matchResult.Winner),
		)

		s.logger.WithFields(logrus.Fields{
			"session_id":        sessionID,
			"match_id":          matchResult.MatchID,
			"original_players":  matchResult.Players,
			"corrected_players": correctedMatchResult.Players,
			"invalid_players":   invalidPlayers,
			"original_winner":   matchResult.Winner,
			"corrected_winner":  correctedMatchResult.Winner,
		}).Warn("Member discrepancy detected - overriding with session data")

		return &correctedMatchResult, nil
	}

	// Validation successful - no corrections needed
	sessionValidationsTotal.WithLabelValues("valid").Inc()
	span.SetAttributes(
		attribute.Bool("validation.success", true),
		attribute.Int("session.members_count", len(sessionMembers)),
	)

	s.logger.WithFields(logrus.Fields{
		"session_id":      sessionID,
		"match_id":        matchResult.MatchID,
		"players":         matchResult.Players,
		"session_members": len(sessionMembers),
	}).Debug("Session validation successful")

	return &correctedMatchResult, nil
}

// MatchResultHistory represents the structure stored in CloudSave
type MatchResultHistory struct {
	MatchResults []*common.KafkaMatchResultMessage `json:"matchResultHistory"`
}

// storeMatchResultInCloudSave stores the match result for all players in AccelByte CloudSave
func (s *Service) storeMatchResultInCloudSave(ctx context.Context, matchResult *common.KafkaMatchResultMessage) error {
	tracer := otel.Tracer("reconciliator")

	// Check if we have the required SDK services
	if s.adminPlayerRecordService == nil {
		return fmt.Errorf("CloudSave services not initialized")
	}

	ctx, span := tracer.Start(ctx, "store_match_result_in_cloudsave",
		trace.WithAttributes(
			attribute.String("match.id", matchResult.MatchID),
			attribute.Int("match.players_count", len(matchResult.Players)),
		),
	)
	defer span.End()

	// Store for each player
	for _, playerID := range matchResult.Players {
		if err := s.storeMatchResultForPlayer(ctx, playerID, matchResult); err != nil {
			s.logger.WithError(err).WithFields(logrus.Fields{
				"player_id": playerID,
				"match_id":  matchResult.MatchID,
			}).Error("Failed to store match result for player")
			// Continue with other players even if one fails
		}
	}

	return nil
}

// storeMatchResultForPlayer stores the match result for a specific player
func (s *Service) storeMatchResultForPlayer(ctx context.Context, playerID string, matchResult *common.KafkaMatchResultMessage) error {
	tracer := otel.Tracer("reconciliator")

	ctx, span := tracer.Start(ctx, "store_match_result_for_player",
		trace.WithAttributes(
			attribute.String("player.id", playerID),
			attribute.String("match.id", matchResult.MatchID),
		),
	)
	defer span.End()

	// Record operation start time
	startTime := time.Now()

	// First, try to get existing match result history
	ctx, getSpan := tracer.Start(ctx, "get_player_match_history")
	getStartTime := time.Now()

	getParams := &admin_player_record.AdminGetPlayerRecordHandlerV1Params{
		Key:       "matchResultHistory",
		Namespace: s.config.AccelByte.Namespace,
		UserID:    playerID,
	}

	existingRecord, err := s.adminPlayerRecordService.AdminGetPlayerRecordHandlerV1Short(getParams)
	getDuration := time.Since(getStartTime).Seconds()
	cloudsaveOperationDuration.WithLabelValues("get").Observe(getDuration)

	var history MatchResultHistory
	if err != nil {
		// Record not found is expected for new players
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "20013") {
			cloudsaveOperationsTotal.WithLabelValues("get", "not_found").Inc()
			s.logger.WithField("player_id", playerID).Debug("No existing match result history found, creating new")
			history = MatchResultHistory{
				MatchResults: []*common.KafkaMatchResultMessage{},
			}
		} else {
			cloudsaveOperationsTotal.WithLabelValues("get", "error").Inc()
			getSpan.RecordError(err)
			getSpan.End()
			return fmt.Errorf("failed to get existing match result history for player %s: %w", playerID, err)
		}
	} else {
		cloudsaveOperationsTotal.WithLabelValues("get", "success").Inc()
		// Parse existing history from CloudSave response
		if existingRecord.Value == nil {
			s.logger.WithField("player_id", playerID).Debug("CloudSave record value is nil, creating new history")
			history = MatchResultHistory{
				MatchResults: []*common.KafkaMatchResultMessage{},
			}
		} else {
			// Try to parse as map[string]interface{} first (new format)
			if valueMap, ok := existingRecord.Value.(map[string]interface{}); ok {
				if matchResultsInterface, exists := valueMap["matchResultHistory"]; exists {
					// Convert interface{} to JSON and then unmarshal to our struct
					matchResultsJSON, err := json.Marshal(matchResultsInterface)
					if err != nil {
						s.logger.WithError(err).WithField("player_id", playerID).Error("Failed to marshal match results from CloudSave map")
						history = MatchResultHistory{
							MatchResults: []*common.KafkaMatchResultMessage{},
						}
					} else {
						var matchResults []*common.KafkaMatchResultMessage
						if err := json.Unmarshal(matchResultsJSON, &matchResults); err != nil {
							s.logger.WithError(err).WithField("player_id", playerID).Error("Failed to unmarshal match results from CloudSave")
							history = MatchResultHistory{
								MatchResults: []*common.KafkaMatchResultMessage{},
							}
						} else {
							history = MatchResultHistory{
								MatchResults: matchResults,
							}
						}
					}
				} else {
					s.logger.WithField("player_id", playerID).Debug("No matchResultHistory key found in CloudSave map, creating new history")
					history = MatchResultHistory{
						MatchResults: []*common.KafkaMatchResultMessage{},
					}
				}
			} else {
				// Fallback: try to parse as JSON bytes (legacy format)
				valueBytes, ok := existingRecord.Value.([]byte)
				if !ok {
					// Try to convert from string to bytes
					if valueStr, ok := existingRecord.Value.(string); ok {
						valueBytes = []byte(valueStr)
					} else {
						s.logger.WithField("player_id", playerID).Error("Failed to convert existing record value to bytes")
						history = MatchResultHistory{
							MatchResults: []*common.KafkaMatchResultMessage{},
						}
					}
				}

				if valueBytes != nil {
					if err := json.Unmarshal(valueBytes, &history); err != nil {
						s.logger.WithError(err).WithField("player_id", playerID).Error("Failed to unmarshal existing match result history from JSON")
						history = MatchResultHistory{
							MatchResults: []*common.KafkaMatchResultMessage{},
						}
					}
				}
			}
		}
	}
	getSpan.End()

	// Add new match result to the beginning of the history
	history.MatchResults = append([]*common.KafkaMatchResultMessage{matchResult}, history.MatchResults...)

	// Keep only the last 10 match results
	if len(history.MatchResults) > 10 {
		history.MatchResults = history.MatchResults[:10]
	}

	// Create map structure for CloudSave instead of marshalling to JSON
	historyMap := map[string]interface{}{
		"matchResultHistory": history.MatchResults,
	}

	// Store the updated history using admin service
	_, putSpan := tracer.Start(ctx, "put_player_match_history")
	putStartTime := time.Now()

	putParams := &admin_player_record.AdminPutPlayerRecordHandlerV1Params{
		Body:      historyMap,
		Key:       "matchResultHistory",
		Namespace: s.config.AccelByte.Namespace,
		UserID:    playerID,
	}

	_, err = s.adminPlayerRecordService.AdminPutPlayerRecordHandlerV1Short(putParams)
	putDuration := time.Since(putStartTime).Seconds()
	cloudsaveOperationDuration.WithLabelValues("put").Observe(putDuration)

	if err != nil {
		cloudsaveOperationsTotal.WithLabelValues("put", "error").Inc()
		putSpan.RecordError(err)
		putSpan.End()
		return fmt.Errorf("failed to store match result history for player %s: %w", playerID, err)
	}

	cloudsaveOperationsTotal.WithLabelValues("put", "success").Inc()
	matchResultsStoredToCloudsave.WithLabelValues(playerID).Inc()
	putSpan.End()

	// Record total operation duration
	totalDuration := time.Since(startTime).Seconds()
	span.SetAttributes(
		attribute.Float64("operation.total_duration", totalDuration),
		attribute.Int("history.total_matches", len(history.MatchResults)),
	)

	s.logger.WithFields(logrus.Fields{
		"player_id":      playerID,
		"match_id":       matchResult.MatchID,
		"history_count":  len(history.MatchResults),
		"total_duration": totalDuration,
	}).Debug("Successfully stored match result history for player")

	return nil
}

// updatePlayerStatistics updates the MMR statistics for all players in the match using AccelByte Statistics service
func (s *Service) updatePlayerStatistics(ctx context.Context, matchResult *common.KafkaMatchResultMessage) error {
	tracer := otel.Tracer("reconciliator")

	ctx, span := tracer.Start(ctx, "update_player_statistics",
		trace.WithAttributes(
			attribute.String("match.id", matchResult.MatchID),
			attribute.Int("players.count", len(matchResult.Players)),
		),
	)
	defer span.End()

	if s.userStatisticService == nil {
		err := fmt.Errorf("user statistic service not initialized")
		span.RecordError(err)
		return err
	}

	for playerID, stats := range matchResult.Stats {
		_, playerSpan := tracer.Start(ctx, "update_player_mmr",
			trace.WithAttributes(
				attribute.String("player.id", playerID),
				attribute.Float64("mmr.value", float64(stats.MMR)),
			),
		)

		strategy := "INCREMENT"
		value := float64(stats.MMR)
		input := &user_statistic.UpdateUserStatItemValueParams{
			Body: &socialclientmodels.StatItemUpdate{
				UpdateStrategy: &strategy,
				Value:          &value,
			},
			Namespace: s.config.AccelByte.Namespace,
			StatCode:  "mmr",
			UserID:    playerID,
		}

		_, err := s.userStatisticService.UpdateUserStatItemValueShort(input)
		if err != nil {
			playerSpan.RecordError(err)
			playerSpan.End()
			s.logger.WithError(err).WithFields(logrus.Fields{
				"player_id": playerID,
				"match_id":  matchResult.MatchID,
				"mmr":       stats.MMR,
			}).Error("Failed to increment player MMR statistic")
			continue // Continue with other players
		}

		playerSpan.End()
		s.logger.WithFields(logrus.Fields{
			"player_id": playerID,
			"match_id":  matchResult.MatchID,
			"mmr_inc":   stats.MMR,
		}).Debug("Successfully incremented player MMR statistic")
	}

	return nil
}

// Discrepancy bitflag constants
const (
	DiscrepancyMatchID         uint64 = 1 << iota // 1
	DiscrepancyPlayersCount                       // 2
	DiscrepancyPlayersMismatch                    // 4
	DiscrepancyWinner                             // 8
	DiscrepancyStartTime                          // 16
	DiscrepancyEndTime                            // 32
	DiscrepancyStatsCount                         // 64
	DiscrepancyPlayerStats                        // 128
)

// MatchComparisonResult represents the result of comparing two match results
type MatchComparisonResult struct {
	Match         bool                            `json:"match"`
	Discrepancies []string                        `json:"discrepancies,omitempty"`
	BitFlags      uint64                          `json:"bit_flags"`
	FirstResult   *common.KafkaMatchResultMessage `json:"first_result"`
	SecondResult  *common.KafkaMatchResultMessage `json:"second_result"`
}

// getStoredMatchResult retrieves the stored match result for a given match ID
func (s *Service) getStoredMatchResult(ctx context.Context, matchID string) (*common.KafkaMatchResultMessage, error) {
	tracer := otel.Tracer("reconciliator")

	ctx, span := tracer.Start(ctx, "get_stored_match_result",
		trace.WithAttributes(
			attribute.String("match.id", matchID),
		),
	)
	defer span.End()

	// Record Redis operation start time
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		redisOperationDuration.WithLabelValues("get").Observe(duration)
		redisOperationsTotal.WithLabelValues("get").Inc()
	}()

	// Create Redis key with namespace
	key := fmt.Sprintf("%s:match:%s", s.namespace, matchID)

	// Get the stored match result
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	jsonData, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No existing result found
		}
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get stored match result: %w", err)
	}

	var matchResult common.KafkaMatchResultMessage
	if err := json.Unmarshal([]byte(jsonData), &matchResult); err != nil {
		s.logger.WithError(err).WithField("match_id", matchID).Error("Failed to unmarshal stored match result")
		span.RecordError(err)
		return nil, fmt.Errorf("failed to unmarshal stored match result: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"match_id": matchID,
		"sender":   matchResult.Sender,
	}).Debug("Retrieved stored match result")

	return &matchResult, nil
}

// storeFirstMatchResult stores the first match result in Redis using SET
func (s *Service) storeFirstMatchResult(ctx context.Context, matchResult *common.KafkaMatchResultMessage) error {
	tracer := otel.Tracer("reconciliator")

	ctx, span := tracer.Start(ctx, "store_first_match_result",
		trace.WithAttributes(
			attribute.String("match.id", matchResult.MatchID),
			attribute.String("match.sender", matchResult.Sender),
		),
	)
	defer span.End()

	// Record Redis operation start time
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		redisOperationDuration.WithLabelValues("set").Observe(duration)
		redisOperationsTotal.WithLabelValues("set").Inc()
	}()

	// Convert to JSON string
	ctx, marshalSpan := tracer.Start(ctx, "marshal_match_result")
	jsonData, err := json.Marshal(matchResult)
	if err != nil {
		marshalSpan.RecordError(err)
		marshalSpan.End()
		return fmt.Errorf("failed to marshal match result: %w", err)
	}
	marshalSpan.End()

	// Create Redis key with namespace
	key := fmt.Sprintf("%s:match:%s", s.namespace, matchResult.MatchID)

	// Store in Redis with TTL using SET
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ctx, setSpan := tracer.Start(ctx, "redis_set_with_ttl")
	if err := s.redisClient.Set(ctx, key, string(jsonData), s.config.Redis.TTL).Err(); err != nil {
		setSpan.RecordError(err)
		setSpan.End()
		return fmt.Errorf("failed to store first match result: %w", err)
	}
	setSpan.End()

	s.logger.WithFields(logrus.Fields{
		"key":    key,
		"sender": matchResult.Sender,
		"ttl":    s.config.Redis.TTL,
		"size":   len(jsonData),
	}).Debug("Stored first match result in Redis")

	return nil
}

// compareMatchResults compares two match results and returns the comparison result
func (s *Service) compareMatchResults(ctx context.Context, first, second *common.KafkaMatchResultMessage) *MatchComparisonResult {
	tracer := otel.Tracer("reconciliator")

	_, span := tracer.Start(ctx, "compare_match_results",
		trace.WithAttributes(
			attribute.String("match.id", first.MatchID),
			attribute.String("first.sender", first.Sender),
			attribute.String("second.sender", second.Sender),
		),
	)
	defer span.End()

	// Record comparison start time
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		matchComparisonDuration.Observe(duration)
	}()

	result := &MatchComparisonResult{
		Match:         true,
		Discrepancies: []string{},
		BitFlags:      0,
		FirstResult:   first,
		SecondResult:  second,
	}

	// Compare basic match info
	if first.MatchID != second.MatchID {
		result.Match = false
		result.BitFlags |= DiscrepancyMatchID
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("match_id mismatch: %s vs %s", first.MatchID, second.MatchID))
	}

	// Compare players (should be the same set)
	if len(first.Players) != len(second.Players) {
		result.Match = false
		result.BitFlags |= DiscrepancyPlayersCount
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("players count mismatch: %d vs %d", len(first.Players), len(second.Players)))
	} else {
		// Check if both have the same players
		firstPlayersSet := make(map[string]bool)
		for _, player := range first.Players {
			firstPlayersSet[player] = true
		}

		for _, player := range second.Players {
			if !firstPlayersSet[player] {
				result.Match = false
				result.BitFlags |= DiscrepancyPlayersMismatch
				result.Discrepancies = append(result.Discrepancies,
					fmt.Sprintf("player %s not found in first result", player))
				break
			}
		}
	}

	// Compare winner
	if first.Winner != second.Winner {
		result.Match = false
		result.BitFlags |= DiscrepancyWinner
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("winner mismatch: %s vs %s", first.Winner, second.Winner))
	}

	// Compare start and end times (allow small tolerance for network delays)
	const timeTolerance = 5 * time.Second
	startTimeDiff := time.Duration(abs(first.StartTime-second.StartTime)) * time.Millisecond
	endTimeDiff := time.Duration(abs(first.EndTime-second.EndTime)) * time.Millisecond

	if startTimeDiff > timeTolerance {
		result.Match = false
		result.BitFlags |= DiscrepancyStartTime
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("start_time mismatch (diff: %v): %d vs %d", startTimeDiff, first.StartTime, second.StartTime))
	}

	if endTimeDiff > timeTolerance {
		result.Match = false
		result.BitFlags |= DiscrepancyEndTime
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("end_time mismatch (diff: %v): %d vs %d", endTimeDiff, first.EndTime, second.EndTime))
	}

	// Compare player stats
	if len(first.Stats) != len(second.Stats) {
		result.Match = false
		result.BitFlags |= DiscrepancyStatsCount
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("stats count mismatch: %d vs %d", len(first.Stats), len(second.Stats)))
	} else {
		for playerID, firstStats := range first.Stats {
			secondStats, exists := second.Stats[playerID]
			if !exists {
				result.Match = false
				result.BitFlags |= DiscrepancyPlayerStats
				result.Discrepancies = append(result.Discrepancies,
					fmt.Sprintf("player %s stats missing in second result", playerID))
				continue
			}

			if firstStats.MMR != secondStats.MMR {
				result.Match = false
				result.BitFlags |= DiscrepancyPlayerStats
				result.Discrepancies = append(result.Discrepancies,
					fmt.Sprintf("player %s MMR mismatch: %d vs %d", playerID, firstStats.MMR, secondStats.MMR))
			}
		}
	}

	// Record metrics
	if result.Match {
		matchComparisonsTotal.WithLabelValues("match").Inc()
		span.SetAttributes(attribute.Bool("comparison.match", true))
	} else {
		matchComparisonsTotal.WithLabelValues("mismatch").Inc()
		span.SetAttributes(
			attribute.Bool("comparison.match", false),
			attribute.Int("comparison.discrepancies_count", len(result.Discrepancies)),
		)
	}

	return result
}

// abs returns the absolute value of an int64
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Stop gracefully shuts down the service
func (s *Service) Stop() error {
	s.logger.Info("Stopping reconciliator service...")

	// Cancel context to stop message consumption
	s.cancel()

	// Close Kafka reader
	if s.kafkaReader != nil {
		if err := s.kafkaReader.Close(); err != nil {
			s.logger.WithError(err).Error("Failed to close Kafka reader")
		}
	}

	// Close Redis client
	if s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			s.logger.WithError(err).Error("Failed to close Redis client")
		}
	}

	s.logger.Info("Reconciliator service stopped")
	return nil
}

// GetHealth returns the health status of the service
func (s *Service) GetHealth(ctx context.Context, req *pb.GetHealthRequest) (*pb.GetHealthResponse, error) {
	tracer := otel.Tracer("reconciliator")

	_, span := tracer.Start(ctx, "health_check")
	defer span.End()

	// Check Redis connection
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := s.redisClient.Ping(ctx).Err(); err != nil {
		span.RecordError(err)
		return &pb.GetHealthResponse{
			Status: "error",
		}, fmt.Errorf("redis health check failed: %w", err)
	}

	return &pb.GetHealthResponse{
		Status: "ok",
	}, nil
}
