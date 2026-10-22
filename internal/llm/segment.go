// Package llm keeps untrusted provider output behind domain validation.
package llm

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

//go:embed prompts/* testdata/*
var assets embed.FS

const PromptVersion = "extract-segment-v1"

var ErrLLMOutputInvalid = errors.New("llm_output_invalid")

var ErrRateLimited = errors.New("provider rate limited")
var ErrTransient = errors.New("provider temporarily unavailable")

type Error struct {
	Code    string                    `json:"code"`
	Path    string                    `json:"path"`
	Reasons []segment.ValidationError `json:"reasons,omitempty"`
}

func (e *Error) Error() string { return e.Code }
func (e *Error) Is(target error) bool {
	return target == ErrLLMOutputInvalid && e.Code == "llm_output_invalid"
}

// Request never contains recipient records. Feedback contains only validator
// codes and paths, not previous raw output or submitted values.
type Request struct {
	Prompt    string
	Schema    json.RawMessage
	Tool      string
	MaxTokens int
	Channels  []string
	Feedback  []segment.ValidationError
}

type Response struct {
	Tool         string          `json:"tool"`
	Input        json.RawMessage `json:"input"`
	Refusal      bool            `json:"refusal"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
}

type Provider interface {
	ExtractSegment(context.Context, Request) (Response, error)
}

// Call is deliberately restricted to metadata; no prompt, brief or output.
type Call struct {
	CampaignID    int64
	Name          string
	PromptVersion string
	Outcome       string
	Errors        []segment.ValidationError
	LatencyMS     int64
	InputTokens   int
	OutputTokens  int
}

type Recorder interface {
	RecordLLMCall(context.Context, Call) error
}

type SegmentGenerator struct {
	Provider Provider
	Recorder Recorder
	Timeout  time.Duration
}

func (g SegmentGenerator) ExtractSegment(ctx context.Context, id int64, brief string) (segment.Validated, error) {
	var zero segment.Validated
	if _, err := (campaign.Draft{Brief: brief, Channels: []string{"sms"}}).Validate(); err != nil {
		return zero, err
	}
	if id < 1 || g.Provider == nil || g.Recorder == nil || g.Timeout <= 0 {
		return zero, &Error{Code: "llm_unavailable", Path: "$"}
	}
	prompt, err := assets.ReadFile("prompts/extract-segment-v1.txt")
	if err != nil {
		return zero, &Error{Code: "llm_unavailable", Path: "$"}
	}
	schema, err := assets.ReadFile("prompts/segment-v1.schema.json")
	if err != nil {
		return zero, &Error{Code: "llm_unavailable", Path: "$"}
	}
	req := Request{Prompt: string(prompt) + "\n<brief>" + html.EscapeString(brief) + "</brief>", Schema: schema, Tool: "extract_segment", MaxTokens: 8192}
	return runCall(ctx, id, "ExtractSegment", PromptVersion, g.Timeout, g.Recorder, req, g.Provider.ExtractSegment, segment.Parse)
}

// runCall shares deadline, repair, retry and audit behavior across both tools.
func runCall[T any](ctx context.Context, id int64, name, version string, timeout time.Duration, recorder Recorder, req Request, call func(context.Context, Request) (Response, error), validate func([]byte) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for repair := 0; repair <= 2; repair++ {
		var reasons []segment.ValidationError
		var validated T
		for transport := 0; transport <= 1; transport++ {
			if ctx.Err() != nil {
				return zero, &Error{Code: "llm_timeout", Path: "$"}
			}
			started := time.Now()
			response, callErr := call(ctx, req)
			outcome := "success"
			retry := false
			switch {
			case ctx.Err() != nil || errors.Is(callErr, context.DeadlineExceeded) || errors.Is(callErr, context.Canceled):
				outcome = "llm_timeout"
			case callErr != nil:
				outcome = "llm_unavailable"
				retry = errors.Is(callErr, ErrRateLimited) || errors.Is(callErr, ErrTransient)
			case response.Refusal:
				outcome = "llm_refused"
			default:
				var validationErr error
				if response.Tool != req.Tool {
					validationErr = segment.Invalid("tool_use_required", "$")
				} else {
					validated, validationErr = validate(response.Input)
				}
				if validationErr != nil {
					outcome = "llm_output_invalid"
					var v *segment.ValidationError
					if errors.As(validationErr, &v) {
						reasons = []segment.ValidationError{*v}
					}
				}
			}
			// Preserve a failure record even after request cancellation, with its
			// own short deadline. No generation/persistence continues afterwards.
			auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			err := recorder.RecordLLMCall(auditCtx, Call{CampaignID: id, Name: name, PromptVersion: version, Outcome: outcome, Errors: reasons, LatencyMS: time.Since(started).Milliseconds(), InputTokens: max(0, response.InputTokens), OutputTokens: max(0, response.OutputTokens)})
			auditCancel()
			if err != nil {
				return zero, &Error{Code: "llm_audit_unavailable", Path: "$"}
			}
			if outcome == "success" {
				if ctx.Err() != nil {
					return zero, &Error{Code: "llm_timeout", Path: "$"}
				}
				return validated, nil
			}
			if retry && transport == 0 {
				timer := time.NewTimer(100 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return zero, &Error{Code: "llm_timeout", Path: "$"}
				case <-timer.C:
				}
				continue
			}
			if outcome != "llm_output_invalid" {
				return zero, &Error{Code: outcome, Path: "$"}
			}
			break
		}
		if repair == 2 {
			return zero, &Error{Code: "llm_output_invalid", Path: "$", Reasons: reasons}
		}
		req.Feedback = append(req.Feedback, reasons...)
	}
	return zero, &Error{Code: "llm_output_invalid", Path: "$"}
}

// Mock is immutable: repair progress comes from the request, so concurrent
// requests never share attempt counters or scenario state.
type Mock struct{ scenario string }

func NewMock(scenario string) (Mock, error) {
	if scenario == "" {
		scenario = "happy"
	}
	switch scenario {
	case "happy", "unknown_field", "extra_keys", "no_tool_use", "refusal", "timeout", "rate_limited", "repair_succeeds", "repair_exhausted":
		return Mock{scenario: scenario}, nil
	default:
		return Mock{}, errors.New("unsupported mock segment scenario")
	}
}

func (m Mock) ExtractSegment(ctx context.Context, req Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if m.scenario == "timeout" {
		<-ctx.Done()
		return Response{}, ctx.Err()
	}
	if m.scenario == "rate_limited" {
		return Response{}, ErrRateLimited
	}
	scenario := m.scenario
	if scenario == "" {
		scenario = "happy"
	}
	if scenario == "repair_succeeds" {
		if len(req.Feedback) == 0 {
			scenario = "unknown_field"
		} else {
			scenario = "happy"
		}
	}
	if scenario == "repair_exhausted" {
		scenario = "unknown_field"
	}
	data, err := assets.ReadFile("testdata/" + scenario + ".json")
	if err != nil {
		return Response{}, errors.New("mock fixture unavailable")
	}
	var response Response
	if err := segment.DecodeStrict(data, &response); err != nil {
		return Response{}, errors.New("invalid mock fixture")
	}
	return response, nil
}
