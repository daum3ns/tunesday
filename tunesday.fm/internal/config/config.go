package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the tunesday.fm server.
type Config struct {
	ListenAddr      string
	BaseURL         string
	DataDir         string
	SQLitePath      string
	SessionSecret   []byte
	SessionSecure   bool
	SessionLifetime time.Duration
	BcryptCost      int

	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr:      getEnv("TUNESDAY_FM_LISTEN_ADDR", ":8080"),
		BaseURL:         os.Getenv("TUNESDAY_FM_BASE_URL"),
		DataDir:         getEnv("TUNESDAY_FM_DATA_DIR", "/data"),
		SessionLifetime: getDuration("TUNESDAY_FM_SESSION_LIFETIME", 7*24*time.Hour),
		BcryptCost:      getInt("TUNESDAY_FM_BCRYPT_COST", 10),
		SessionSecure:   getBool("TUNESDAY_FM_SESSION_SECURE", true),
		SMTPHost:        os.Getenv("TUNESDAY_FM_SMTP_HOST"),
		SMTPPort:        getInt("TUNESDAY_FM_SMTP_PORT", 587),
		SMTPUser:        os.Getenv("TUNESDAY_FM_SMTP_USER"),
		SMTPPass:        os.Getenv("TUNESDAY_FM_SMTP_PASS"),
		SMTPFrom:        getEnv("TUNESDAY_FM_SMTP_FROM", "noreply@tunesday.fm"),
	}

	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("TUNESDAY_FM_BASE_URL is required")
	}

	secret := os.Getenv("TUNESDAY_FM_SESSION_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("TUNESDAY_FM_SESSION_SECRET is required")
	}
	cfg.SessionSecret = []byte(secret)

	if cfg.SMTPHost == "" || cfg.SMTPUser == "" || cfg.SMTPPass == "" {
		return nil, fmt.Errorf("TUNESDAY_FM_SMTP_HOST, TUNESDAY_FM_SMTP_USER, and TUNESDAY_FM_SMTP_PASS are required")
	}

	cfg.SQLitePath = getEnv("TUNESDAY_FM_SQLITE_PATH", cfg.DataDir+"/tunesday.db")

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return n
}

func getBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	switch v {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return defaultValue
	}
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultValue
	}
	return d
}
