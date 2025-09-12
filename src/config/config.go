package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port        string
	DBPath      string
	BaseURL     string
	StorageType string
	PostgresURL string
}

func Load() *Config {
	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		DBPath:      getEnv("DB_PATH", "./sqs.db"),
		StorageType: getEnv("STORAGE_TYPE", "sqlite"),
		PostgresURL: getEnv("POSTGRES_URL", ""),
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:" + cfg.Port
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
