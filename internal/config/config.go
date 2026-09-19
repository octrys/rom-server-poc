// Package config loads the game-server configuration from the environment,
// applying sensible defaults for local development.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime settings for the game server.
type Config struct {
	// GameListenAddr is the raw-TCP game socket the client connects to.
	GameListenAddr string
	// RegionInternalURL is the base URL of the rom-api region service, used to
	// validate a client's sessionKey (GET/POST /internal/sessions/:key).
	RegionInternalURL string
	// ConsumeSession picks single-use validation (POST .../consume) over the
	// reusable GET when true.
	ConsumeSession bool
	// DatabaseURL is the Postgres DSN for persistence.
	DatabaseURL string
	// WorldID is the world this server instance hosts (must match the session's).
	WorldID int
	// TickInterval is the world simulation step period.
	TickInterval time.Duration
	// AuthTimeout bounds each call to the region service.
	AuthTimeout time.Duration
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	tickHz, err := envInt("ROM_TICK_HZ", 20)
	if err != nil {
		return Config{}, err
	}
	if tickHz <= 0 {
		return Config{}, fmt.Errorf("config: ROM_TICK_HZ must be positive, got %d", tickHz)
	}
	worldID, err := envInt("ROM_WORLD_ID", 1)
	if err != nil {
		return Config{}, err
	}

	return Config{
		GameListenAddr:    env("ROM_GAME_LISTEN", ":17701"),
		RegionInternalURL: env("ROM_REGION_URL", "http://127.0.0.1:8002"),
		ConsumeSession:    envBool("ROM_CONSUME_SESSION", false),
		DatabaseURL:       env("ROM_DATABASE_URL", "postgres://rom:rom@127.0.0.1:5432/rom?sslmode=disable"),
		WorldID:           worldID,
		TickInterval:      time.Second / time.Duration(tickHz),
		AuthTimeout:       5 * time.Second,
	}, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer: %w", key, err)
	}
	return n, nil
}

func envBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
