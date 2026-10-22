package campaign

type Page struct {
	Campaigns  []Campaign `json:"campaigns"`
	NextBefore *int64     `json:"next_before"`
}
