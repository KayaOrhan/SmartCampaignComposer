package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

var errProviderOutput = errors.New("invalid provider tool response")

// toolJSON keeps at most 32 KiB. Re-reading this bounded prefix lets encoding/json
// handle object structure; only incomplete string decoding needs special care.
// Final strict domain validation, not these provisional deltas, decides validity.
// ponytail: re-read <=32 KiB per delta; use an incremental tokenizer if profiling warrants it.
type toolJSON struct {
	body    []byte
	emitted map[string]string
}
type jsonFrame struct {
	path                []string
	object, keyExpected bool
	key                 string
}

func (d *toolJSON) feed(fragment string, emit func(string, string, string) error) error {
	if len(d.body)+len(fragment) > segment.MaxJSONBytes {
		return errProviderOutput
	}
	d.body = append(d.body, fragment...)
	if emit == nil {
		return nil
	}
	if d.emitted == nil {
		d.emitted = map[string]string{}
	}
	// seen is per parse pass, so repeated reads are harmless but duplicate
	// keys in the same document are rejected before they can rewrite a field.
	seen := map[string]bool{}
	decoder := json.NewDecoder(bytes.NewReader(d.body))
	decoder.UseNumber()
	stack := []jsonFrame{}
	pathForValue := func() []string {
		if len(stack) == 0 {
			return nil
		}
		f := stack[len(stack)-1]
		name := f.key
		if !f.object {
			name = "#"
		}
		return append(append([]string(nil), f.path...), name)
	}
	consume := func() {
		if len(stack) > 0 && stack[len(stack)-1].object {
			stack[len(stack)-1].keyExpected = true
		}
	}
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if err != nil {
			if len(stack) > 0 && stack[len(stack)-1].object && !stack[len(stack)-1].keyExpected {
				raw := bytes.TrimSpace(d.body[offset:])
				raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte(":")))
				if len(raw) > 0 && raw[0] == '"' {
					value, prefixErr := stringPrefix(raw)
					if prefixErr != nil {
						return prefixErr
					}
					return d.display(pathForValue(), value, false, seen, emit)
				}
			}
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "unexpected EOF") {
				return nil
			}
			return errProviderOutput
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				path := pathForValue()
				consume()
				if len(stack) >= 16 {
					return errProviderOutput
				}
				stack = append(stack, jsonFrame{path: path, object: delimiter == '{', keyExpected: delimiter == '{'})
			case '}', ']':
				if len(stack) == 0 {
					return errProviderOutput
				}
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].keyExpected {
			name, ok := token.(string)
			if !ok {
				return errProviderOutput
			}
			stack[len(stack)-1].key = name
			stack[len(stack)-1].keyExpected = false
			continue
		}
		if value, ok := token.(string); ok {
			if err := d.display(pathForValue(), value, true, seen, emit); err != nil {
				return err
			}
		}
		consume()
	}
}

func (d *toolJSON) display(path []string, value string, complete bool, seen map[string]bool, emit func(string, string, string) error) error {
	if len(path) != 2 || !((path[0] == "sms" && path[1] == "body") || (path[0] == "email" && (path[1] == "body" || path[1] == "subject"))) {
		return nil
	}
	key := path[0] + "." + path[1]
	if seen[key] {
		return errProviderOutput
	}
	if complete {
		seen[key] = true
	}
	old := d.emitted[key]
	if !strings.HasPrefix(value, old) {
		return errProviderOutput
	}
	text := value[len(old):]
	for len(text) > 0 {
		n := min(len(text), 1024)
		for n > 0 && !utf8.ValidString(text[:n]) {
			n--
		}
		if n == 0 {
			return errProviderOutput
		}
		if err := emit(path[0], path[1], text[:n]); err != nil {
			return err
		}
		text = text[n:]
	}
	d.emitted[key] = value
	return nil
}

// stringPrefix returns only fully decoded characters. An unfinished escape,
// UTF-8 sequence or surrogate pair stays buffered until the next fragment.
func stringPrefix(raw []byte) (string, error) {
	var out strings.Builder
	for i := 1; i < len(raw); {
		b := raw[i]
		if b == '"' {
			return out.String(), nil
		}
		if b < 0x20 {
			return "", errProviderOutput
		}
		if b != '\\' {
			if !utf8.FullRune(raw[i:]) {
				return out.String(), nil
			}
			r, size := utf8.DecodeRune(raw[i:])
			if r == utf8.RuneError && size == 1 {
				return "", errProviderOutput
			}
			out.WriteRune(r)
			i += size
			continue
		}
		if i+1 >= len(raw) {
			return out.String(), nil
		}
		switch raw[i+1] {
		case '"', '\\', '/':
			out.WriteByte(raw[i+1])
			i += 2
		case 'b':
			out.WriteByte('\b')
			i += 2
		case 'f':
			out.WriteByte('\f')
			i += 2
		case 'n':
			out.WriteByte('\n')
			i += 2
		case 'r':
			out.WriteByte('\r')
			i += 2
		case 't':
			out.WriteByte('\t')
			i += 2
		case 'u':
			if i+6 > len(raw) {
				return out.String(), nil
			}
			v, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
			if err != nil {
				return "", errProviderOutput
			}
			r := rune(v)
			i += 6
			if r >= 0xD800 && r <= 0xDBFF {
				if i+6 > len(raw) {
					return out.String(), nil
				}
				if raw[i] != '\\' || raw[i+1] != 'u' {
					return "", errProviderOutput
				}
				low, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
				if err != nil || low < 0xDC00 || low > 0xDFFF {
					return "", errProviderOutput
				}
				r = utf16.DecodeRune(r, rune(low))
				i += 6
			} else if utf16.IsSurrogate(r) {
				return "", errProviderOutput
			}
			out.WriteRune(r)
		default:
			return "", errProviderOutput
		}
	}
	return out.String(), nil
}
