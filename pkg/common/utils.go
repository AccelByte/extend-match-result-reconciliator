// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel/propagation"

	"github.com/sirupsen/logrus"
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func GetEnvInt(key string, fallback int) int {
	str := GetEnv(key, strconv.Itoa(fallback))
	val, err := strconv.Atoi(str)
	if err != nil {
		return fallback
	}

	return val
}

func GetBasePath() string {
	basePath := os.Getenv("BASE_PATH")
	if basePath == "" {
		logrus.Fatalf("BASE_PATH envar is not set or empty")
	}
	if !strings.HasPrefix(basePath, "/") {
		logrus.Fatalf("BASE_PATH envar is invalid, no leading '/' found. Valid example: /basePath")
	}

	return basePath
}

func GetNamespace() string {
	return GetEnv("AB_NAMESPACE", "accelbyte")
}

func IsAuthEnabled() bool {
	return strings.ToLower(GetEnv("PLUGIN_GRPC_SERVER_AUTH_ENABLED", "true")) == "true"
}

// GetTextMapPropagator returns the global TextMapPropagator
func GetTextMapPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		b3.New(),
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// KafkaHeaderCarrier implements propagation.TextMapCarrier for Kafka headers
type KafkaHeaderCarrier []kafka.Header

func (c *KafkaHeaderCarrier) Get(key string) string {
	for _, header := range *c {
		if header.Key == key {
			return string(header.Value)
		}
	}
	return ""
}

func (c *KafkaHeaderCarrier) Set(key, value string) {
	for i, header := range *c {
		if header.Key == key {
			(*c)[i].Value = []byte(value)
			return
		}
	}
	*c = append(*c, kafka.Header{Key: key, Value: []byte(value)})
}

func (c *KafkaHeaderCarrier) Keys() []string {
	keys := make([]string, len(*c))
	for i, header := range *c {
		keys[i] = header.Key
	}
	return keys
}

// ExtractTraceContextFromKafkaHeaders extracts trace context from Kafka headers
func ExtractTraceContextFromKafkaHeaders(ctx context.Context, headers []kafka.Header) context.Context {
	propagator := GetTextMapPropagator()
	carrier := KafkaHeaderCarrier(headers)

	extractedCtx := propagator.Extract(ctx, &carrier)

	return extractedCtx
}

// InjectTraceContextIntoKafkaHeaders injects trace context into Kafka headers
func InjectTraceContextIntoKafkaHeaders(ctx context.Context, headers *[]kafka.Header) {
	propagator := GetTextMapPropagator()
	carrier := KafkaHeaderCarrier(*headers)

	propagator.Inject(ctx, &carrier)

	// Update the original headers slice with the modified carrier
	*headers = []kafka.Header(carrier)
}
