package config

import (
	"errors"
	"strconv"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/delivery"
)

type Worker struct {
	DatabaseURL, ProviderURL                string
	DBTimeout, ProviderTimeout, Lease, Poll time.Duration
	Concurrency                             int
}

func LoadWorker(get func(string) string) (Worker, error) {
	base, err := Load(func(k string) string {
		if k == "LLM_PROVIDER" {
			return "mock"
		}
		if k == "SEED_DATA" {
			return "false"
		}
		return get(k)
	})
	if err != nil {
		return Worker{}, err
	}
	c := Worker{DatabaseURL: base.DatabaseURL, DBTimeout: base.DBTimeout, ProviderURL: get("PROVIDER_BASE_URL"), ProviderTimeout: 3 * time.Second, Lease: 30 * time.Second, Poll: 100 * time.Millisecond, Concurrency: 1}
	if c.ProviderURL == "" {
		c.ProviderURL = "http://mockprovider:8081"
	}
	for _, field := range []struct {
		name    string
		target  *time.Duration
		maximum time.Duration
	}{{"PROVIDER_TIMEOUT", &c.ProviderTimeout, 30 * time.Second}, {"WORKER_LEASE", &c.Lease, 5 * time.Minute}, {"WORKER_POLL", &c.Poll, 10 * time.Second}} {
		if raw := get(field.name); raw != "" {
			*field.target, err = time.ParseDuration(raw)
			if err != nil || *field.target <= 0 || *field.target > field.maximum {
				return Worker{}, errors.New("invalid worker duration")
			}
		}
	}
	if raw := get("WORKER_CONCURRENCY"); raw != "" {
		c.Concurrency, err = strconv.Atoi(raw)
		if err != nil || c.Concurrency < 1 || c.Concurrency > 32 {
			return Worker{}, errors.New("invalid worker concurrency")
		}
	}
	if c.Lease < c.ProviderTimeout+2*c.DBTimeout+time.Second {
		return Worker{}, errors.New("worker lease must exceed provider and database deadlines")
	}
	client, err := delivery.NewClient(c.ProviderURL, c.ProviderTimeout)
	if err != nil {
		return Worker{}, err
	}
	client.Close()
	return c, nil
}

type Mock struct {
	DatabaseURL, Address string
	DBTimeout            time.Duration
	Seed                 int64
	Rate5xx, Rate429     float64
	Latency              time.Duration
}

func LoadMock(get func(string) string) (Mock, error) {
	base, err := Load(func(k string) string {
		if k == "LLM_PROVIDER" {
			return "mock"
		}
		if k == "SEED_DATA" {
			return "false"
		}
		return get(k)
	})
	if err != nil {
		return Mock{}, err
	}
	c := Mock{DatabaseURL: base.DatabaseURL, Address: base.Address, DBTimeout: base.DBTimeout, Seed: 1}
	if get("HTTP_ADDR") == "" {
		c.Address = ":8081"
	}
	if raw := get("FAULT_SEED"); raw != "" {
		c.Seed, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Mock{}, errors.New("invalid mock seed")
		}
	}
	for _, f := range []struct {
		name   string
		target *float64
	}{{"FAULT_RATE_5XX", &c.Rate5xx}, {"FAULT_RATE_429", &c.Rate429}} {
		if raw := get(f.name); raw != "" {
			*f.target, err = strconv.ParseFloat(raw, 64)
			if err != nil || !(*f.target >= 0 && *f.target <= 1) {
				return Mock{}, errors.New("invalid mock fault rate")
			}
		}
	}
	if c.Rate5xx+c.Rate429 > 1 {
		return Mock{}, errors.New("mock fault rates exceed one")
	}
	if raw := get("FAULT_LATENCY_P95_MS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 10000 {
			return Mock{}, errors.New("invalid mock latency")
		}
		c.Latency = time.Duration(n) * time.Millisecond
	}
	return c, nil
}
