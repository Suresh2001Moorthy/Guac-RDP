package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	BindAddress string
	WebRoot     string
	LogLevel    string
}

func Load() *Config {
	cfg := &Config{
		BindAddress: getEnvOrDefault("RDP_BIND_ADDR", ":8081"),
		WebRoot:     getEnvOrDefault("RDP_WEB_ROOT", resolveWebRoot()),
		LogLevel:    getEnvOrDefault("RDP_LOG_LEVEL", "INFO"),
	}
	return cfg
}

// resolveWebRoot returns an absolute path to the web directory,
// relative to the executable's location.
func resolveWebRoot() string {
	// Try relative to working directory first
	if abs, err := filepath.Abs("web"); err == nil {
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs
		}
	}
	// Fallback to the original hardcoded path for backward compatibility
	return `C:\Guac-RDP\web`
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
