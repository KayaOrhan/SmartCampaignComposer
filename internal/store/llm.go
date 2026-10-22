package store

import (
	"context"
	"encoding/json"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func (s Campaigns) RecordLLMCall(ctx context.Context, call llm.Call) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	if call.Errors == nil {
		call.Errors = []segment.ValidationError{}
	}
	encoded, err := json.Marshal(call.Errors)
	if err != nil {
		return ErrUnavailable
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO app.llm_calls(campaign_id,call_name,prompt_version,outcome,validation_errors,latency_ms,input_tokens,output_tokens) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8)`, call.CampaignID, call.Name, call.PromptVersion, call.Outcome, string(encoded), call.LatencyMS, call.InputTokens, call.OutputTokens)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
