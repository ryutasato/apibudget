package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/ryutasato/apibudget"
)

func main() {
	configPath := envOrDefault("APIBUDGET_CONFIG", "apibudget.yaml")
	addr := envOrDefault("APIBUDGET_ADDR", ":8080")
	storeType := envOrDefault("APIBUDGET_STORE_TYPE", "memory")
	logLevelStr := envOrDefault("APIBUDGET_LOG_LEVEL", "info")

	logLevel := parseLogLevel(logLevelStr)

	var opts []apibudget.ManagerOption
	opts = append(opts, apibudget.WithLogLevel(logLevel))

	// Store設定
	switch strings.ToLower(storeType) {
	case "redis":
		redisAddr := envOrDefault("APIBUDGET_REDIS_ADDR", "localhost:6379")
		redisPassword := os.Getenv("APIBUDGET_REDIS_PASSWORD")
		redisDBStr := envOrDefault("APIBUDGET_REDIS_DB", "0")
		redisTLSStr := envOrDefault("APIBUDGET_REDIS_TLS", "false")

		redisDB, err := strconv.Atoi(redisDBStr)
		if err != nil {
			log.Fatalf("invalid APIBUDGET_REDIS_DB: %v", err)
		}
		redisTLS := strings.ToLower(redisTLSStr) == "true"

		var redisOpts []apibudget.RedisOption
		if redisPassword != "" {
			redisOpts = append(redisOpts, apibudget.WithRedisPassword(redisPassword))
		}
		redisOpts = append(redisOpts, apibudget.WithRedisDB(redisDB))
		if redisTLS {
			redisOpts = append(redisOpts, apibudget.WithRedisTLS(true))
		}

		store, err := apibudget.NewRedisStore(redisAddr, redisOpts...)
		if err != nil {
			log.Fatalf("failed to create Redis store: %v", err)
		}
		defer store.Close()
		opts = append(opts, apibudget.WithStore(store))
	case "memory":
		// default, no action needed
	default:
		log.Fatalf("unknown store type: %q (supported: memory, redis)", storeType)
	}

	manager, err := apibudget.NewBudgetManagerFromYAML(configPath, opts...)
	if err != nil {
		log.Fatalf("failed to create BudgetManager: %v", err)
	}

	server := apibudget.NewServer(manager, addr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
		cancel()
	}()

	fmt.Printf("Starting apibudget server on %s\n", addr)
	if err := server.Start(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseLogLevel(s string) apibudget.LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return apibudget.LogLevelDebug
	case "info":
		return apibudget.LogLevelInfo
	case "warn":
		return apibudget.LogLevelWarn
	case "error":
		return apibudget.LogLevelError
	case "silent":
		return apibudget.LogLevelSilent
	default:
		return apibudget.LogLevelInfo
	}
}
