package mockprovider

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/delivery"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

var keyPattern = regexp.MustCompile(`^campaign:[1-9][0-9]*:user:[1-9][0-9]*:(sms|email)$`)

type Provider struct {
	DB               *sql.DB
	Timeout          time.Duration
	Seed             int64
	Rate5xx, Rate429 float64
	Latency          time.Duration
}

func (p Provider) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /messages", p.message)
	mux.HandleFunc("GET /debug/messages", p.counts)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), p.Timeout)
		defer cancel()
		if p.DB.PingContext(ctx) != nil {
			respond(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	})
	return mux
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (p Provider) message(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		respond(w, 415, map[string]string{"error": "content_type"})
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
	if err != nil {
		respond(w, 413, map[string]string{"error": "body_limit"})
		return
	}
	var payload delivery.Payload
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) || len(key) > 128 || segment.DecodeStrict(raw, &payload) != nil || !valid(payload) || !strings.HasSuffix(key, ":"+payload.Channel) {
		respond(w, 422, map[string]string{"error": "invalid_payload"})
		return
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		respond(w, 422, map[string]string{"error": "invalid_payload"})
		return
	}
	hash := sha256.Sum256(canonical)
	ctx, cancel := context.WithTimeout(r.Context(), p.Timeout)
	defer cancel()
	status, id, err := p.accept(ctx, key, payload.Channel, hex.EncodeToString(hash[:]))
	if err != nil {
		respond(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	if status == 429 {
		w.Header().Set("Retry-After", "1")
	}
	if status != 200 {
		respond(w, status, map[string]string{"error": "mock_failure"})
		return
	}
	respond(w, 200, map[string]string{"message_id": id})
}

func valid(m delivery.Payload) bool {
	if !utf8.ValidString(m.Body) || strings.TrimSpace(m.Body) == "" || strings.ContainsRune(m.Body, 0) {
		return false
	}
	if m.Channel == "sms" {
		return m.Subject == nil && utf8.RuneCountInString(m.Body) <= 160 && strings.HasPrefix(m.Destination, "mock-phone-") && len(m.Destination) <= 254
	}
	if m.Channel != "email" || m.Subject == nil || !strings.HasSuffix(m.Destination, "@example.test") || len(m.Destination) > 254 {
		return false
	}
	return utf8.ValidString(*m.Subject) && !strings.ContainsRune(*m.Subject, 0) && strings.TrimSpace(*m.Subject) != "" && utf8.RuneCountInString(*m.Subject) <= 80 && utf8.RuneCountInString(m.Body) <= 2000
}

func (p Provider) accept(ctx context.Context, key, channel, hash string) (status int, id string, result error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", errors.New("mock storage unavailable")
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			result = errors.New("mock rollback failed")
		}
	}()
	// A channel row lock makes deduplication, fault attempts and the independent
	// provider rolling-window check atomic, including across provider restarts.
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT channel FROM app.delivery_channels WHERE channel=$1 FOR UPDATE`, channel).Scan(&locked); err != nil {
		return 0, "", errors.New("mock storage unavailable")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM app.mock_calls WHERE channel=$1 AND started_at<=clock_timestamp()-interval '1 second'`, channel); err != nil {
		return 0, "", errors.New("mock storage unavailable")
	}
	var recent int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app.mock_calls WHERE channel=$1`, channel).Scan(&recent); err != nil {
		return 0, "", errors.New("mock storage unavailable")
	}
	if recent >= 100 {
		if tx.Commit() != nil {
			return 0, "", errors.New("mock storage unavailable")
		}
		return 429, "", nil
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app.mock_calls(channel) VALUES($1)`, channel); err != nil {
		return 0, "", errors.New("mock storage unavailable")
	}
	var previous string
	err = tx.QueryRowContext(ctx, `SELECT payload_hash,message_id FROM app.mock_acceptances WHERE idempotency_key=$1`, key).Scan(&previous, &id)
	if err == nil {
		if previous != hash {
			return 409, "", nil
		}
		if tx.Commit() != nil {
			return 0, "", errors.New("mock storage unavailable")
		}
		return 200, id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, "", errors.New("mock storage unavailable")
	}
	var attempt int
	if err = tx.QueryRowContext(ctx, `INSERT INTO app.mock_attempts(idempotency_key,attempts) VALUES($1,1) ON CONFLICT(idempotency_key) DO UPDATE SET attempts=app.mock_attempts.attempts+1 RETURNING attempts`, key).Scan(&attempt); err != nil {
		return 0, "", errors.New("mock storage unavailable")
	}
	entropy := sha256.Sum256([]byte(strconv.FormatInt(p.Seed, 10) + ":" + key + ":" + strconv.Itoa(attempt)))
	value := float64(binary.BigEndian.Uint64(entropy[:8])>>11) / float64(uint64(1)<<53)
	status = 200
	switch {
	case value < p.Rate429:
		status = 429
	case value < p.Rate429+p.Rate5xx:
		status = 503
	}
	if p.Latency > 0 {
		if err = delivery.Wait(ctx, time.Duration(binary.BigEndian.Uint64(entropy[8:16])%uint64(p.Latency+1))); err != nil {
			return 0, "", errors.New("mock cancelled")
		}
	}
	if status == 200 {
		sum := sha256.Sum256([]byte(key))
		id = "mock-" + hex.EncodeToString(sum[:])
		if _, err = tx.ExecContext(ctx, `INSERT INTO app.mock_acceptances(idempotency_key,payload_hash,channel,message_id) VALUES($1,$2,$3,$4)`, key, hash, channel, id); err != nil {
			return 0, "", errors.New("mock storage unavailable")
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, "", errors.New("mock storage unavailable")
	}
	return status, id, nil
}

func (p Provider) counts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), p.Timeout)
	defer cancel()
	rows, err := p.DB.QueryContext(ctx, `SELECT channel,count(*) FROM app.mock_acceptances GROUP BY channel`)
	if err != nil {
		respond(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	counts := map[string]int64{"sms": 0, "email": 0}
	var total int64
	for rows.Next() {
		var ch string
		var n int64
		if rows.Scan(&ch, &n) != nil {
			_ = rows.Close()
			respond(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		counts[ch] = n
		total += n
	}
	rowErr, closeErr := rows.Err(), rows.Close()
	if rowErr != nil || closeErr != nil {
		respond(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	respond(w, 200, map[string]any{"total": total, "by_channel": counts})
}
