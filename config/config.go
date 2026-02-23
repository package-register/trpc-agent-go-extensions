package config

import (
	"os"
)

// Config represents the global configuration for the extensions.
type Config struct {
	Telemetry TelemetryConfig
	Log       LogConfig
}

// TelemetryConfig holds tracing and monitoring settings.
type TelemetryConfig struct {
	Langfuse LangfuseConfig
}

// LangfuseConfig specifically for Langfuse backend.
type LangfuseConfig struct {
	SecretKey string
	PublicKey string
	Host      string
	Insecure  bool
}

// LogConfig for logging settings.
type LogConfig struct {
	Level string // debug, info, warn, error
}

// LoadFromEnv creates a Config by reading environment variables.
// This is the default way to initialize configuration.
func LoadFromEnv() *Config {
	return &Config{
		Telemetry: TelemetryConfig{
			Langfuse: LangfuseConfig{
				SecretKey: getEnv("LANGFUSE_SECRET_KEY", ""),
				PublicKey: getEnv("LANGFUSE_PUBLIC_KEY", ""),
				Host:      getEnv("LANGFUSE_HOST", "localhost:3000"),
				Insecure:  getEnv("LANGFUSE_INSECURE", "false") == "true",
			},
		},
		Log: LogConfig{
			Level: getEnv("LOG_LEVEL", "info"),
		},
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
