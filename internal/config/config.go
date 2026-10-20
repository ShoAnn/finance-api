package config

import "os"

// Config holds runtime settings, all sourced from the environment.
type Config struct {
	Port        string
	DatabaseURL string
}

// Load reads configuration from the environment, falling back to
// development-friendly defaults that match compose.yaml.
func Load() Config {
	return Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://finance:finance@localhost:5432/finance?sslmode=disable"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
