package campaign

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

type SMS struct {
	Body string `json:"body"`
}
type Email struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
type Messages struct {
	SMS   *SMS   `json:"sms,omitempty"`
	Email *Email `json:"email,omitempty"`
}

type MessageValidation struct {
	Revision int64   `json:"revision"`
	Audience Preview `json:"audience"`
}

type SavedMessages struct {
	CampaignID int64              `json:"campaign_id"`
	Revision   int64              `json:"revision"`
	Status     string             `json:"status"`
	Messages   json.RawMessage    `json:"messages"`
	Validation *MessageValidation `json:"validation"`
}

// ValidatedMessages keeps validated templates immutable outside this package.
type ValidatedMessages struct{ canonical json.RawMessage }

func (v ValidatedMessages) JSON() json.RawMessage {
	return append(json.RawMessage(nil), v.canonical...)
}

type RenderedMessage struct {
	Subject *string `json:"subject"`
	Body    string  `json:"body"`
}

// Render performs literal, nonrecursive substitution. Profile text containing
// braces is data, never a second template or executable expression.
func (v ValidatedMessages) Render(channel, firstName, city string) (RenderedMessage, error) {
	var m Messages
	if err := json.Unmarshal(v.canonical, &m); err != nil {
		return RenderedMessage{}, segment.Invalid("invalid_message", "$")
	}
	for _, field := range []struct {
		text  string
		limit int
	}{{firstName, 80}, {city, 120}} {
		if !utf8.ValidString(field.text) || strings.ContainsRune(field.text, 0) || utf8.RuneCountInString(field.text) < 1 || utf8.RuneCountInString(field.text) > field.limit {
			return RenderedMessage{}, segment.Invalid("invalid_audience_bounds", "audience")
		}
	}
	replacer := strings.NewReplacer("{{first_name}}", firstName, "{{city}}", city)
	result := RenderedMessage{}
	switch {
	case channel == "sms" && m.SMS != nil:
		result.Body = replacer.Replace(m.SMS.Body)
	case channel == "email" && m.Email != nil:
		subject := replacer.Replace(m.Email.Subject)
		result.Subject = &subject
		result.Body = replacer.Replace(m.Email.Body)
	default:
		return result, segment.Invalid("channel_required", channel)
	}
	limit := 160
	if channel == "email" {
		limit = 2000
		if strings.TrimSpace(*result.Subject) == "" || utf8.RuneCountInString(*result.Subject) > 80 {
			return RenderedMessage{}, segment.Invalid("invalid_message", "email.subject")
		}
	}
	if strings.TrimSpace(result.Body) == "" || utf8.RuneCountInString(result.Body) > limit {
		return RenderedMessage{}, segment.Invalid("invalid_message", channel+".body")
	}
	return result, nil
}

// ValidateMessages measures every repeated placeholder with BOTH independent
// maxima together. Zero-count channels use schema bounds, never zero lengths.
func ValidateMessages(body []byte, channels []string, bounds map[string]Bounds) (ValidatedMessages, error) {
	var zero ValidatedMessages
	var messages Messages
	if err := segment.DecodeStrict(body, &messages); err != nil {
		return zero, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return zero, segment.Invalid("invalid_shape", "$")
	}
	selected := map[string]bool{}
	for _, ch := range channels {
		if (ch != "sms" && ch != "email") || selected[ch] {
			return zero, segment.Invalid("invalid_channels", "channels")
		}
		selected[ch] = true
	}
	if len(selected) == 0 {
		return zero, segment.Invalid("invalid_channels", "channels")
	}
	for ch := range fields {
		if !selected[ch] {
			return zero, segment.Invalid("unexpected_channel", "$")
		}
	}
	for _, ch := range channels {
		b, ok := bounds[ch]
		if !ok || b.Count < 0 || b.MaxFirstName < 0 || b.MaxFirstName > 80 || b.MaxCity < 0 || b.MaxCity > 120 || (b.Count > 0 && (b.MaxFirstName == 0 || b.MaxCity == 0)) {
			return zero, segment.Invalid("invalid_audience_bounds", "audience")
		}
		if b.Count == 0 {
			b.MaxFirstName, b.MaxCity = 80, 120
		}
		if ch == "sms" {
			if messages.SMS == nil {
				return zero, segment.Invalid("channel_required", "sms")
			}
			if err := validateTemplate(messages.SMS.Body, b, 160, "sms.body"); err != nil {
				return zero, err
			}
		} else {
			if messages.Email == nil {
				return zero, segment.Invalid("channel_required", "email")
			}
			if err := validateTemplate(messages.Email.Subject, b, 80, "email.subject"); err != nil {
				return zero, err
			}
			if err := validateTemplate(messages.Email.Body, b, 2000, "email.body"); err != nil {
				return zero, err
			}
		}
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		return zero, segment.Invalid("invalid_shape", "$")
	}
	return ValidatedMessages{canonical: encoded}, nil
}

func validateTemplate(text string, b Bounds, limit int, path string) error {
	if strings.TrimSpace(text) == "" || strings.ContainsRune(text, 0) {
		return segment.Invalid("invalid_message", path)
	}
	length := 0
	for len(text) > 0 {
		i := strings.IndexAny(text, "{}")
		if i < 0 {
			length += utf8.RuneCountInString(text)
			break
		}
		length += utf8.RuneCountInString(text[:i])
		text = text[i:]
		if !strings.HasPrefix(text, "{{") {
			return segment.Invalid("invalid_placeholder", path)
		}
		end := strings.Index(text, "}}")
		if end < 0 {
			return segment.Invalid("invalid_placeholder", path)
		}
		switch text[2:end] {
		case "first_name":
			length += b.MaxFirstName
		case "city":
			length += b.MaxCity
		default:
			return segment.Invalid("unknown_variable", path)
		}
		text = text[end+2:]
	}
	if length > limit {
		return segment.Invalid("message_too_long", path)
	}
	return nil
}
