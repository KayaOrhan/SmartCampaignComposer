package segment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxJSONBytes = 32768

// MaxDocumentBytes bounds canonical and PostgreSQL JSONB representations.
const MaxDocumentBytes = 256 * 1024

type ValidationError struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

func (e *ValidationError) Error() string { return e.Code + " at " + e.Path }
func Invalid(code, path string) error    { return &ValidationError{Code: code, Path: path} }

// DecodeStrict rejects duplicate keys, trailing values and invalid UTF-8 as well
// as unknown struct fields. It never includes submitted values in errors.
func DecodeStrict(body []byte, target any) error {
	return decodeStrict(body, target, MaxJSONBytes)
}

func decodeStrict(body []byte, target any, maxBytes int) error {
	if len(body) > maxBytes {
		return Invalid("body_too_large", "$")
	}
	if !utf8.Valid(body) {
		return Invalid("invalid_json", "$")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 16 {
			return errors.New("too deep")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, container := token.(json.Delim)
		if !container {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate key")
				}
				seen[name] = true
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := visit(0); err != nil {
		return Invalid("invalid_json", "$")
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return Invalid("invalid_json", "$")
	}
	if err := exactFields(body, reflect.TypeOf(target), "$"); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return Invalid("invalid_shape", "$")
	}
	return nil
}

// encoding/json matches struct keys case-insensitively by default. Enforce the
// exact documented spelling before decoding, including nested conditions.
func exactFields(body []byte, t reflect.Type, path string) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[json.RawMessage]() {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
			return Invalid("invalid_shape", path)
		}
		allowed := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag != "" && tag != "-" {
				allowed[tag] = f.Type
			}
		}
		for name, value := range fields {
			fieldType, ok := allowed[name]
			if !ok {
				return Invalid("unknown_field", path)
			}
			if err := exactFields(value, fieldType, path+"."+name); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if t.Elem().Kind() != reflect.Struct {
			return nil
		}
		var values []json.RawMessage
		if err := json.Unmarshal(body, &values); err != nil {
			return Invalid("invalid_shape", path)
		}
		for i, value := range values {
			if err := exactFields(value, t.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

type condition struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}
type document struct {
	Version int         `json:"version"`
	All     []condition `json:"all"`
}
type clause struct {
	field, op string
	values    []string
	days      int
}

// Validated has no publicly writable fields. The zero value cannot compile.
type Validated struct {
	clauses   []clause
	canonical json.RawMessage
}

func (v Validated) JSON() json.RawMessage { return append(json.RawMessage(nil), v.canonical...) }

var languageCode = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})*$`)

func Parse(body []byte) (Validated, error) {
	return parse(body, MaxJSONBytes)
}

// ParseDocument revalidates stored or canonical JSON without reapplying the
// HTTP ingress limit. All structural and domain checks remain identical.
func ParseDocument(body []byte) (Validated, error) {
	return parse(body, MaxDocumentBytes)
}

func parse(body []byte, maxBytes int) (Validated, error) {
	var doc document
	if err := decodeStrict(body, &doc, maxBytes); err != nil {
		return Validated{}, err
	}
	if doc.Version != 1 {
		return Validated{}, Invalid("unsupported_version", "version")
	}
	if len(doc.All) < 1 || len(doc.All) > 8 {
		return Validated{}, Invalid("condition_count", "all")
	}
	v := Validated{}
	for i, c := range doc.All {
		prefix := fmt.Sprintf("all[%d]", i)
		cl := clause{field: c.Field, op: c.Op}
		switch c.Field {
		case "city", "language":
			if c.Op != "in" {
				return Validated{}, Invalid("unsupported_operator", prefix+".op")
			}
			if err := json.Unmarshal(c.Value, &cl.values); err != nil || len(cl.values) < 1 || len(cl.values) > 20 {
				return Validated{}, Invalid("invalid_list", prefix+".value")
			}
			for j, value := range cl.values {
				limit := 120
				if c.Field == "language" {
					limit = 16
				}
				if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > limit || strings.ContainsRune(value, 0) {
					return Validated{}, Invalid("invalid_string", prefix+".value")
				}
				if c.Field == "city" {
					cl.values[j] = NormalizeCity(value)
				} else {
					cl.values[j] = strings.ToLower(strings.TrimSpace(value))
					if !languageCode.MatchString(cl.values[j]) {
						return Validated{}, Invalid("invalid_language", prefix+".value")
					}
				}
			}
			encoded, err := json.Marshal(cl.values)
			if err != nil {
				return Validated{}, Invalid("invalid_list", prefix+".value")
			}
			c.Value = encoded
		case "cart_added", "purchase":
			if c.Op != "within_days" && !(c.Field == "purchase" && c.Op == "none_within_days") {
				return Validated{}, Invalid("unsupported_operator", prefix+".op")
			}
			if err := json.Unmarshal(c.Value, &cl.days); err != nil || cl.days < 1 || cl.days > 365 {
				return Validated{}, Invalid("invalid_days", prefix+".value")
			}
		default:
			return Validated{}, Invalid("unsupported_field", prefix+".field")
		}
		doc.All[i] = c
		v.clauses = append(v.clauses, cl)
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return Validated{}, Invalid("invalid_shape", "$")
	}
	v.canonical = encoded
	return v, nil
}

// Query selects only consented recipients. Identifiers and SQL operators are
// fixed in this code; all caller/model values become bound parameters.
func (v Validated) Query(channel string, asOf time.Time) (string, []any, error) {
	if len(v.clauses) == 0 {
		return "", nil, Invalid("segment_required", "segment")
	}
	if channel != "sms" && channel != "email" {
		return "", nil, Invalid("invalid_channel", "channel")
	}
	if asOf.IsZero() {
		return "", nil, Invalid("invalid_time", "as_of")
	}
	args := []any{channel}
	query := `SELECT u.id, u.first_name, u.city FROM app.users u WHERE EXISTS (SELECT 1 FROM app.consents c WHERE c.user_id=u.id AND c.channel=$1 AND c.granted=true)`
	bind := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	for _, c := range v.clauses {
		switch c.field {
		case "city":
			query += " AND u.city_norm = ANY(" + bind(append([]string(nil), c.values...)) + "::text[])"
		case "language":
			query += " AND u.language = ANY(" + bind(append([]string(nil), c.values...)) + "::text[])"
		case "cart_added", "purchase":
			start := bind(asOf.UTC().Add(-time.Duration(c.days) * 24 * time.Hour))
			end := bind(asOf.UTC())
			if c.field == "cart_added" {
				query += " AND EXISTS (SELECT 1 FROM app.cart_events e WHERE e.user_id=u.id AND e.added_at >= " + start + " AND e.added_at < " + end + ")"
			} else {
				not := ""
				if c.op == "none_within_days" {
					not = "NOT "
				}
				query += " AND " + not + "EXISTS (SELECT 1 FROM app.orders o WHERE o.user_id=u.id AND o.placed_at >= " + start + " AND o.placed_at < " + end + ")"
			}
		}
	}
	return query, args, nil
}
