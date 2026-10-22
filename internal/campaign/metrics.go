package campaign

type CallMetric struct {
	Call               string
	Outcome            string
	Count              int64
	ValidationFailures int64
}

type Metrics struct {
	Deliveries map[string]DeliveryCounts
	Calls      []CallMetric
}
