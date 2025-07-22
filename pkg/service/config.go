// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"extend-match-result-reconciliator/pkg/common"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the configuration for the reconciliator service
type Config struct {
	// Kafka configuration
	Kafka KafkaConfig `json:"kafka"`

	// Redis configuration
	Redis RedisConfig `json:"redis"`

	// Logging configuration
	Log LogConfig `json:"log"`

	// Service configuration
	Service ServiceConfig `json:"service"`

	// AccelByte SDK configuration
	AccelByte AccelByteConfig `json:"accelByte"`
}

// KafkaConfig holds Kafka-related configuration
type KafkaConfig struct {
	Brokers  []string `json:"brokers"`
	Topic    string   `json:"topic"`
	GroupID  string   `json:"groupId"`
	MinBytes int      `json:"minBytes"`
	MaxBytes int      `json:"maxBytes"`
	MaxWait  int      `json:"maxWait"` // in seconds
	Username string   `json:"username"`
	Password string   `json:"password"`
}

// RedisConfig holds Redis-related configuration
type RedisConfig struct {
	Addr     string        `json:"addr"`
	Password string        `json:"password"`
	DB       int           `json:"db"`
	TTL      time.Duration `json:"ttl"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"` // json or text
}

// ServiceConfig holds general service configuration
type ServiceConfig struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Environment     string `json:"environment"`
	ShutdownTimeout int    `json:"shutdownTimeout"` // in seconds
}

// AccelByteConfig holds AccelByte SDK configuration
type AccelByteConfig struct {
	BaseURL      string `json:"baseURL"`
	ClientID     string `json:"clientID"`
	ClientSecret string `json:"clientSecret"`
	Namespace    string `json:"namespace"`
	Enabled      bool   `json:"enabled"`
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		Kafka: KafkaConfig{
			Brokers:  getKafkaBrokers(),
			Topic:    getEnvOrDefault("KAFKA_TOPIC", common.GetNamespace()+".match"),
			GroupID:  getEnvOrDefault("KAFKA_GROUP_ID", "reconciliator-group"),
			MinBytes: getEnvIntOrDefault("KAFKA_MIN_BYTES", 10240),    // 10KB
			MaxBytes: getEnvIntOrDefault("KAFKA_MAX_BYTES", 10485760), // 10MB
			MaxWait:  getEnvIntOrDefault("KAFKA_MAX_WAIT_SEC", 5),
			Username: os.Getenv("KAFKA_USERNAME"),
			Password: os.Getenv("KAFKA_PASSWORD"),
		},
		Redis: RedisConfig{
			Addr:     getEnvOrDefault("REDIS_ADDR", "localhost:6379"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       getEnvIntOrDefault("REDIS_DB", 0),
			TTL:      getEnvDurationOrDefault("REDIS_TTL_SECONDS", 300), // 5 minutes
		},
		Log: LogConfig{
			Level:  getEnvOrDefault("LOG_LEVEL", "info"),
			Format: getEnvOrDefault("LOG_FORMAT", "text"),
		},
		Service: ServiceConfig{
			Name:            getEnvOrDefault("SERVICE_NAME", "reconciliator"),
			Version:         getEnvOrDefault("SERVICE_VERSION", "1.0.0"),
			Environment:     getEnvOrDefault("ENVIRONMENT", "development"),
			ShutdownTimeout: getEnvIntOrDefault("SHUTDOWN_TIMEOUT_SEC", 30),
		},
		AccelByte: AccelByteConfig{
			BaseURL:      getEnvOrDefault("AB_BASE_URL", "https://demo.accelbyte.io"),
			ClientID:     getEnvOrDefault("AB_CLIENT_ID", ""),
			ClientSecret: getEnvOrDefault("AB_CLIENT_SECRET", ""),
			Namespace:    getEnvOrDefault("AB_NAMESPACE", common.GetNamespace()),
			Enabled:      getEnvBoolOrDefault("AB_SESSION_VALIDATION_ENABLED", false),
		},
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate Kafka configuration
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}
	if c.Kafka.Topic == "" {
		return fmt.Errorf("kafka topic is required")
	}
	if c.Kafka.GroupID == "" {
		return fmt.Errorf("kafka group ID is required")
	}

	// Validate Redis configuration
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis address is required")
	}
	if c.Redis.DB < 0 || c.Redis.DB > 15 {
		return fmt.Errorf("redis DB must be between 0-15, got: %d", c.Redis.DB)
	}
	if c.Redis.TTL <= 0 {
		return fmt.Errorf("redis TTL must be positive")
	}

	// Validate log configuration
	validLogLevels := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
	if !contains(validLogLevels, c.Log.Level) {
		return fmt.Errorf("invalid log level: %s, must be one of: %s", c.Log.Level, strings.Join(validLogLevels, ", "))
	}

	validLogFormats := []string{"json", "text"}
	if !contains(validLogFormats, c.Log.Format) {
		return fmt.Errorf("invalid log format: %s, must be one of: %s", c.Log.Format, strings.Join(validLogFormats, ", "))
	}

	// Validate AccelByte configuration (only if enabled)
	if c.AccelByte.Enabled {
		if c.AccelByte.BaseURL == "" {
			return fmt.Errorf("AccelByte base URL is required when session validation is enabled")
		}
		if c.AccelByte.ClientID == "" {
			return fmt.Errorf("AccelByte client ID is required when session validation is enabled")
		}
		if c.AccelByte.ClientSecret == "" {
			return fmt.Errorf("AccelByte client secret is required when session validation is enabled")
		}
		if c.AccelByte.Namespace == "" {
			return fmt.Errorf("AccelByte namespace is required when session validation is enabled")
		}
	}

	return nil
}

// getKafkaBrokers parses the KAFKA_BROKERS environment variable
func getKafkaBrokers() []string {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return []string{}
	}

	brokerList := strings.Split(brokers, ",")
	for i, broker := range brokerList {
		brokerList[i] = strings.TrimSpace(broker)
	}

	return brokerList
}

// getEnvOrDefault returns the environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault returns the environment variable as int or default
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvDurationOrDefault returns the environment variable as duration (in seconds) or default
func getEnvDurationOrDefault(key string, defaultSeconds int) time.Duration {
	if value := os.Getenv(key); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return time.Duration(defaultSeconds) * time.Second
}

// getEnvBoolOrDefault returns the environment variable as bool or default
func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetRequiredEnvVars returns a list of required environment variables
func GetRequiredEnvVars() []string {
	return []string{
		"KAFKA_BROKERS",
	}
}

// GetOptionalEnvVars returns a list of optional environment variables with their defaults
func GetOptionalEnvVars() map[string]string {
	return map[string]string{
		"KAFKA_TOPIC":                   common.GetNamespace() + ".match",
		"KAFKA_GROUP_ID":                "reconciliator-group",
		"KAFKA_MIN_BYTES":               "10240",
		"KAFKA_MAX_BYTES":               "10485760",
		"KAFKA_MAX_WAIT_SEC":            "1",
		"REDIS_ADDR":                    "localhost:6379",
		"REDIS_PASSWORD":                "",
		"REDIS_DB":                      "0",
		"REDIS_TTL_SECONDS":             "300", // 5 minutes
		"LOG_LEVEL":                     "info",
		"LOG_FORMAT":                    "json",
		"SERVICE_NAME":                  "reconciliator",
		"SERVICE_VERSION":               "1.0.0",
		"ENVIRONMENT":                   "development",
		"SHUTDOWN_TIMEOUT_SEC":          "30",
		"AB_BASE_URL":                   "https://demo.accelbyte.io",
		"AB_CLIENT_ID":                  "",
		"AB_CLIENT_SECRET":              "",
		"AB_NAMESPACE":                  common.GetNamespace(),
		"AB_SESSION_VALIDATION_ENABLED": "false",
	}
}
