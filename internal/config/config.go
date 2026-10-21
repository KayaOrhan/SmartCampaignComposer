package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"
)

type Config struct {
	Address     string
	DatabaseURL string
	DBTimeout   time.Duration
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
	return c, nil
}
