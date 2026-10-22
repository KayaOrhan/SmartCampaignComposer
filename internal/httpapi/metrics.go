package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

func (a campaignAPI) metrics(w http.ResponseWriter, r *http.Request) {
	m, err := a.store.Metrics(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	var b strings.Builder
	b.WriteString("# HELP deliveries Current persisted delivery jobs by channel and state.\n# TYPE deliveries gauge\n")
	for _, channel := range []string{"sms", "email"} {
		c := m.Deliveries[channel]
		for _, v := range []struct {
			name  string
			count int64
		}{{"pending", c.Pending}, {"in_flight", c.InFlight}, {"sent", c.Sent}, {"dead", c.Dead}, {"skipped", c.Skipped}} {
			fmt.Fprintf(&b, "deliveries{channel=%q,status=%q} %d\n", channel, v.name, v.count)
		}
	}
	b.WriteString("# HELP queue_depth Pending and in-flight jobs, including retry waits.\n# TYPE queue_depth gauge\n")
	for _, channel := range []string{"sms", "email"} {
		c := m.Deliveries[channel]
		fmt.Fprintf(&b, "queue_depth{channel=%q} %d\n", channel, c.Pending+c.InFlight)
	}
	b.WriteString("# HELP llm_calls_total Audited provider attempts, including repairs and retries.\n# TYPE llm_calls_total counter\n")
	// Fixed labels prevent accidental data leakage and unbounded cardinality.
	for _, call := range []string{"ExtractSegment", "GenerateMessages"} {
		for _, outcome := range []string{"success", "llm_output_invalid", "llm_timeout", "llm_unavailable", "llm_refused"} {
			var count int64
			for _, v := range m.Calls {
				if v.Call == call && v.Outcome == outcome {
					count += v.Count
				}
			}
			fmt.Fprintf(&b, "llm_calls_total{call=%q,outcome=%q} %d\n", call, outcome, count)
		}
	}
	b.WriteString("# HELP llm_validation_failures_total Recorded schema or domain validation errors, not failed campaigns.\n# TYPE llm_validation_failures_total counter\n")
	for _, call := range []string{"ExtractSegment", "GenerateMessages"} {
		var count int64
		for _, v := range m.Calls {
			if v.Call == call {
				count += v.ValidationFailures
			}
		}
		fmt.Fprintf(&b, "llm_validation_failures_total{call=%q} %d\n", call, count)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write([]byte(b.String())); err != nil {
		slog.Warn("metrics response write failed")
	}
}
