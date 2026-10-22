package config

import "testing"

func TestDeliveryConfig(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://app:app@db:5432/test"}
	get := func(k string) string { return base[k] }
	if _, err := LoadWorker(get); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"WORKER_CONCURRENCY": "0", "WORKER_LEASE": "1s", "PROVIDER_BASE_URL": "http://example.com:80", "WORKER_POLL": "0s", "PROVIDER_TIMEOUT": "31s"} {
		base[key] = value
		if _, err := LoadWorker(get); err == nil {
			t.Fatal("invalid worker setting", key)
		}
		delete(base, key)
	}
	if _, err := LoadMock(get); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"FAULT_RATE_5XX": "NaN", "FAULT_RATE_429": "1.1", "FAULT_LATENCY_P95_MS": "-1", "FAULT_SEED": "bad"} {
		base[key] = value
		if _, err := LoadMock(get); err == nil {
			t.Fatal("invalid mock setting", key)
		}
		delete(base, key)
	}
}
