package llm

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"time"
	"unicode/utf8"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

const MessagePromptVersion = "generate-messages-v1"

// Event deltas are provisional plain text, never HTML or approvable content.
type Event struct {
	Attempt int    `json:"attempt"`
	Channel string `json:"channel,omitempty"`
	Field   string `json:"field,omitempty"`
	Text    string `json:"text,omitempty"`
}
type Emit func(string, Event) error
type MessageProvider interface {
	GenerateMessages(context.Context, Request, func(string, string, string) error) (Response, error)
}
type MessageGenerator struct {
	Provider MessageProvider
	Recorder Recorder
	Timeout  time.Duration
}

func (g MessageGenerator) GenerateMessages(ctx context.Context, id int64, brief string, channels []string, audience campaign.Preview, emit Emit) (campaign.ValidatedMessages, error) {
	var zero campaign.ValidatedMessages
	if _, err := (campaign.Draft{Brief: brief, Channels: channels}).Validate(); err != nil {
		return zero, err
	}
	if id < 1 || g.Provider == nil || g.Recorder == nil || g.Timeout <= 0 || emit == nil {
		return zero, &Error{Code: "llm_unavailable", Path: "$"}
	}
	prompt, err := assets.ReadFile("prompts/generate-messages-v1.txt")
	if err != nil {
		return zero, &Error{Code: "llm_unavailable", Path: "$"}
	}
	schema, err := assets.ReadFile("prompts/messages-v1.schema.json")
	if err != nil {
		return zero, &Error{Code: "llm_unavailable", Path: "$"}
	}
	summary, err := json.Marshal(audience)
	if err != nil {
		return zero, &Error{Code: "llm_unavailable", Path: "$"}
	}
	selectedChannels, err := json.Marshal(channels)
	if err != nil {
		return zero, &Error{Code: "llm_unavailable", Path: "$"}
	}
	req := Request{Prompt: string(prompt) + "\n<brief>" + html.EscapeString(brief) + "</brief>\n<channels>" + string(selectedChannels) + "</channels>\n<audience>" + string(summary) + "</audience>", Schema: schema, Tool: "generate_messages", MaxTokens: 4096, Channels: append([]string(nil), channels...)}
	attempt := 0
	call := func(ctx context.Context, req Request) (Response, error) {
		attempt++
		bytes := 0
		if err := emit("attempt", Event{Attempt: attempt}); err != nil {
			return Response{}, err
		}
		return g.Provider.GenerateMessages(ctx, req, func(channel, field, text string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			selected := false
			for _, ch := range channels {
				if ch == channel {
					selected = true
				}
			}
			bytes += len(text)
			if !selected || (field != "body" && !(channel == "email" && field == "subject")) || !utf8.ValidString(text) || len(text) == 0 || len(text) > 1024 || bytes > 32768 {
				return errProviderOutput
			}
			return emit("delta", Event{Attempt: attempt, Channel: channel, Field: field, Text: text})
		})
	}
	return runCall(ctx, id, "GenerateMessages", MessagePromptVersion, g.Timeout, g.Recorder, req, call, func(body []byte) (campaign.ValidatedMessages, error) {
		return campaign.ValidateMessages(body, channels, audience.ByChannel)
	})
}

type MessageMock struct {
	scenario string
	delay    time.Duration
}

func NewMessageMock(scenario string, delay time.Duration) (MessageMock, error) {
	if scenario == "" {
		scenario = "happy"
	}
	if delay < 0 || delay > time.Second {
		return MessageMock{}, errors.New("mock chunk delay must be between 0s and 1s")
	}
	switch scenario {
	case "happy", "sms_too_long", "subject_too_long", "unknown_field", "unknown_variable", "extra_keys", "no_tool_use", "refusal", "timeout", "rate_limited", "repair_succeeds", "repair_exhausted":
		return MessageMock{scenario: scenario, delay: delay}, nil
	default:
		return MessageMock{}, errors.New("unsupported mock message scenario")
	}
}

func (m MessageMock) GenerateMessages(ctx context.Context, req Request, emit func(string, string, string) error) (Response, error) {
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
	if scenario == "repair_succeeds" {
		if len(req.Feedback) == 0 {
			scenario = "unknown_variable"
		} else {
			scenario = "happy"
		}
	}
	if scenario == "repair_exhausted" {
		scenario = "unknown_variable"
	}
	data, err := assets.ReadFile("testdata/messages_" + scenario + ".json")
	if err != nil {
		return Response{}, errors.New("mock message fixture unavailable")
	}
	var response Response
	if err := segment.DecodeStrict(data, &response); err != nil {
		return Response{}, errors.New("invalid mock message fixture")
	}
	if response.Refusal || response.Tool != "generate_messages" {
		return response, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.Input, &raw); err != nil {
		return Response{}, errors.New("invalid mock message document")
	}
	// Fixture selection respects requested channels. An extra unknown key is
	// kept so the final strict validator, not this mock, rejects it.
	for _, channel := range []string{"sms", "email"} {
		selected := false
		for _, ch := range req.Channels {
			if ch == channel {
				selected = true
			}
		}
		if !selected {
			delete(raw, channel)
		}
	}
	response.Input, err = json.Marshal(raw)
	if err != nil {
		return Response{}, errors.New("encode mock messages failed")
	}
	for _, channel := range []string{"sms", "email"} {
		var fields map[string]string
		if value, ok := raw[channel]; ok {
			if err := json.Unmarshal(value, &fields); err != nil {
				return Response{}, errors.New("invalid mock message fields")
			}
		}
		for _, field := range []string{"subject", "body"} {
			text, ok := fields[field]
			if !ok {
				continue
			}
			runes := []rune(text)
			for start := 0; start < len(runes); start += 8 {
				if m.delay > 0 {
					timer := time.NewTimer(m.delay)
					select {
					case <-ctx.Done():
						timer.Stop()
						return Response{}, ctx.Err()
					case <-timer.C:
					}
				}
				if err := ctx.Err(); err != nil {
					return Response{}, err
				}
				if err := emit(channel, field, string(runes[start:min(start+8, len(runes))])); err != nil {
					return Response{}, err
				}
			}
		}
	}
	return response, nil
}
