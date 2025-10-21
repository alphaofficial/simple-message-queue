package config

import (
	"os"
)

type Config struct {
	Port          string
	BaseURL       string
	StorageType   string
	SQLiteDBPath  string
	PostgresURL   string
	PostgresHost  string
	PostgresPort  string
	PostgresUser  string
	PostgresPass  string
	PostgresDB    string
	AdminUsername string
	AdminPassword string
}

func Load() *Config {
	cfg := &Config{
		Port:          getEnv("PORT", "8080"),
		StorageType:   getEnv("STORAGE_ADAPTER", "sqlite"),
		SQLiteDBPath:  getEnv("SQLITE_DB_PATH", "./sqs.db"),
		PostgresURL:   getEnv("DATABASE_URL", ""),
		PostgresHost:  getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:  getEnv("POSTGRES_PORT", "5432"),
		PostgresUser:  getEnv("POSTGRES_USER", ""),
		PostgresPass:  getEnv("POSTGRES_PASSWORD", ""),
		PostgresDB:    getEnv("POSTGRES_DB", ""),
		AdminUsername: getEnv("ADMIN_USERNAME", ""),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),
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
