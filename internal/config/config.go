package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"
)

type Config struct {
	Address                string
	DatabaseURL            string
	DBTimeout              time.Duration
	SeedData               bool
	LLMProvider            string
	LLMMockScenario        string
	LLMMockMessageScenario string
	LLMMockChunkDelay      time.Duration
	LLMTimeout             time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{Address: getenv("HTTP_ADDR"), DatabaseURL: getenv("DATABASE_URL"), DBTimeout: 2 * time.Second}
	if c.Address == "" {
		c.Address = ":8080"
	}
	_, port, err := net.SplitHostPort(c.Address)
	n, parseErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || n < 1 || n > 65535 {
		return Config{}, errors.New("HTTP_ADDR must contain a valid host and port")
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" {
		return Config{}, errors.New("DATABASE_URL must be a PostgreSQL URL with host and database")
	}
	if raw := getenv("DB_TIMEOUT"); raw != "" {
		c.DBTimeout, err = time.ParseDuration(raw)
		if err != nil || c.DBTimeout <= 0 || c.DBTimeout > 30*time.Second {
			return Config{}, errors.New("DB_TIMEOUT must be positive and at most 30s")
		}
	}
	if raw := getenv("SEED_DATA"); raw != "" {
		if raw != "true" && raw != "false" {
			return Config{}, errors.New("SEED_DATA must be true or false")
		}
		c.SeedData = raw == "true"
	}

	c.LLMProvider = getenv("LLM_PROVIDER")
	if c.LLMProvider == "" {
		c.LLMProvider = "mock"
	}
	if c.LLMProvider != "mock" && c.LLMProvider != "anthropic" {
		return Config{}, errors.New("LLM_PROVIDER must be mock or anthropic")
	}
	if c.LLMProvider == "anthropic" && getenv("APP_ENV") == "test" {
		return Config{}, errors.New("real LLM provider forbidden in test")
	}
	c.LLMMockScenario = getenv("LLM_MOCK_SCENARIO")
	c.LLMTimeout = 10 * time.Second
	if raw := getenv("LLM_TIMEOUT"); raw != "" {
		c.LLMTimeout, err = time.ParseDuration(raw)
		if err != nil || c.LLMTimeout <= 0 || c.LLMTimeout > 30*time.Second {
			return Config{}, errors.New("LLM_TIMEOUT must be positive and at most 30s")
		}
	}

	c.LLMMockMessageScenario = getenv("LLM_MOCK_MESSAGE_SCENARIO")
	c.LLMMockChunkDelay = 10 * time.Millisecond
	if raw := getenv("LLM_MOCK_CHUNK_DELAY"); raw != "" {
		c.LLMMockChunkDelay, err = time.ParseDuration(raw)
		if err != nil || c.LLMMockChunkDelay < 0 || c.LLMMockChunkDelay > time.Second {
			return Config{}, errors.New("LLM_MOCK_CHUNK_DELAY must be between 0s and 1s")
		}
	}
	return c, nil
}
