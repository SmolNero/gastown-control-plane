package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr              string
	DatabaseURL           string
	AutoMigrate           bool
	MaxEventBytes         int64
	MaxSnapshotBytes      int64
	RateLimitPerMinute    int
	EventRetentionDays    int
	SnapshotRetentionDays int
	PruneInterval         time.Duration
	Version               string
	AgentDownloadBaseURL  string
}

func FromEnv() Config {
	return Config{
		HTTPAddr:              getEnv("GTCP_HTTP_ADDR", ":8080"),
		DatabaseURL:           getEnv("GTCP_DATABASE_URL", "postgres://gtcp:gtcp@localhost:5432/gtcp?sslmode=disable"),
		AutoMigrate:           getEnvBool("GTCP_AUTO_MIGRATE", false),
		MaxEventBytes:         getEnvInt64("GTCP_MAX_EVENT_BYTES", 1<<20),
		MaxSnapshotBytes:      getEnvInt64("GTCP_MAX_SNAPSHOT_BYTES", 4<<20),
		RateLimitPerMinute:    getEnvInt("GTCP_RATE_LIMIT_PER_MINUTE", 600),
		EventRetentionDays:    getEnvInt("GTCP_EVENT_RETENTION_DAYS", 30),
		SnapshotRetentionDays: getEnvInt("GTCP_SNAPSHOT_RETENTION_DAYS", 30),
		PruneInterval:         getEnvDuration("GTCP_PRUNE_INTERVAL", time.Hour),
		Version:               getEnv("GTCP_VERSION", "dev"),
		AgentDownloadBaseURL:  getEnv("GTCP_AGENT_DOWNLOAD_BASE_URL", ""),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
