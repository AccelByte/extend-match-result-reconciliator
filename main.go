// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"extend-match-result-reconciliator/pkg/common"
	"extend-match-result-reconciliator/pkg/service"

	"github.com/prometheus/client_golang/prometheus"
	prometheusCollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	reconciliatorServiceName = common.GetEnv("RECONCILIATOR_SERVICE_NAME", "ExtendMatchResultReconciliator")
)

func main() {
	// Load configuration
	config, err := service.LoadConfig()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to load configuration")
	}

	// Initialize logger
	logger := setupLogger(config)

	logger.WithFields(logrus.Fields{
		"service":     config.Service.Name,
		"version":     config.Service.Version,
		"environment": config.Service.Environment,
	}).Info("Starting reconciliator service")

	// Print configuration (without sensitive data)
	printConfiguration(config, logger)

	// Set up OpenTelemetry
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set Tracer Provider
	tracerProvider, err := common.NewTracerProvider(reconciliatorServiceName)
	if err != nil {
		logger.Fatalf("Failed to create tracer provider: %v", err)
	}
	otel.SetTracerProvider(tracerProvider)
	defer func(ctx context.Context) {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			logger.Fatal(err)
		}
	}(ctx)

	// Set Text Map Propagator
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			b3.New(),
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// Create reconciliator service
	reconciliatorService := service.NewService(logger, config)

	// Setup HTTP server for health checks with OTel instrumentation
	healthServer := setupHealthServer(reconciliatorService, logger)

	// Start the service
	if err := reconciliatorService.Start(); err != nil {
		logger.WithError(err).Fatal("Failed to start reconciliator service")
	}

	// Start health check server
	go func() {
		logger.Info("Starting health check server on :8080")
		logger.Infof("Metrics endpoint: (:8080/metrics)")
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Error("Health check server failed")
		}
	}()

	// Wait for interrupt signal
	waitForShutdown(reconciliatorService, healthServer, config, logger)
}

// setupLogger configures the logger based on configuration
func setupLogger(config *service.Config) *logrus.Logger {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(config.Log.Level)
	if err != nil {
		logger.WithError(err).Warn("Invalid log level, using info")
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Set log format
	if config.Log.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	}

	return logger
}

// printConfiguration prints the current configuration (without sensitive data)
func printConfiguration(config *service.Config, logger *logrus.Logger) {
	logger.Info("Configuration loaded:")
	logger.WithFields(logrus.Fields{
		"kafka_brokers":   config.Kafka.Brokers,
		"kafka_topic":     config.Kafka.Topic,
		"kafka_group_id":  config.Kafka.GroupID,
		"redis_addr":      config.Redis.Addr,
		"redis_db":        config.Redis.DB,
		"redis_ttl":       config.Redis.TTL.String(),
		"log_level":       config.Log.Level,
		"log_format":      config.Log.Format,
		"service_name":    config.Service.Name,
		"service_version": config.Service.Version,
		"environment":     config.Service.Environment,
	}).Info("Service configuration")
}

// setupHealthServer creates an HTTP server for health checks with OTel instrumentation
func setupHealthServer(service *service.Service, logger *logrus.Logger) *http.Server {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, span := trace.SpanFromContext(r.Context()).TracerProvider().Tracer("reconciliator").Start(r.Context(), "health_check")
		defer span.End()

		if err := service.HealthCheck(); err != nil {
			logger.WithError(err).Error("Health check failed")
			span.RecordError(err)
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"unhealthy","error":"%s"}`, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"healthy"}`)
	})

	// Ready check endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		_, span := trace.SpanFromContext(r.Context()).TracerProvider().Tracer("reconciliator").Start(r.Context(), "ready_check")
		defer span.End()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ready"}`)
	})

	// Create combined metrics registry
	combinedRegistry := prometheus.NewRegistry()

	// Register standard collectors
	combinedRegistry.MustRegister(
		prometheusCollectors.NewGoCollector(),
		prometheusCollectors.NewProcessCollector(prometheusCollectors.ProcessCollectorOpts{}),
	)

	// Register service's custom metrics
	serviceMetrics := service.GetMetricsRegistry()
	if serviceMetrics != nil {
		combinedRegistry.MustRegister(serviceMetrics)
	}

	// Metrics endpoint with Prometheus handler
	mux.Handle("/metrics", promhttp.HandlerFor(combinedRegistry, promhttp.HandlerOpts{}))

	// Wrap the mux with OTel HTTP instrumentation
	otelHandler := otelhttp.NewHandler(mux, "reconciliator_http")

	return &http.Server{
		Addr:    ":8080",
		Handler: otelHandler,
	}
}

// waitForShutdown waits for interrupt signal and gracefully shuts down the service
func waitForShutdown(service *service.Service, healthServer *http.Server, config *service.Config, logger *logrus.Logger) {
	// Create channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigChan
	logger.WithField("signal", sig.String()).Info("Received shutdown signal")

	// Create shutdown context with timeout
	shutdownTimeout := time.Duration(config.Service.ShutdownTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown health server
	logger.Info("Shutting down health check server...")
	if err := healthServer.Shutdown(ctx); err != nil {
		logger.WithError(err).Error("Failed to shutdown health check server")
	}

	// Shutdown reconciliator service
	logger.Info("Shutting down reconciliator service...")
	if err := service.Stop(); err != nil {
		logger.WithError(err).Error("Failed to shutdown reconciliator service")
	}

	logger.Info("Reconciliator service shutdown completed")
}
