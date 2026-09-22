package config

import "os"

type Config struct {
	AppEnv         string
	HTTPPort       string
	DatabaseURL    string
	FrontendOrigin string
}

func Load() Config {
	return Config{
		AppEnv:         getEnv("APP_ENV", "development"),
		HTTPPort:       getEnv("HTTP_PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://sppr_user:sppr_password@localhost:5433/sppr_db?sslmode=disable"),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
