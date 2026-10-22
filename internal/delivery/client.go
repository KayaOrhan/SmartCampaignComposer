package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	base    string
	timeout time.Duration
	http    *http.Client
}

func NewClient(base string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u == nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || timeout <= 0 || timeout > 30*time.Second {
		return nil, errors.New("invalid mock provider configuration")
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid mock provider port")
	}
	host := u.Hostname()
	if host != "mockprovider" && host != "127.0.0.1" && host != "::1" {
		return nil, errors.New("provider host is not allowlisted")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: timeout}).DialContext, MaxIdleConns: 16, MaxIdleConnsPerHost: 16, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: timeout}
	return &Client{base: strings.TrimSuffix(base, "/"), timeout: timeout, http: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) Send(ctx context.Context, j Job) Outcome {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	body, err := json.Marshal(j.Payload)
	if err != nil {
		return Outcome{Status: "dead", Code: "invalid_payload"}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/messages", bytes.NewReader(body))
	if err != nil {
		return Outcome{Status: "dead", Code: "invalid_request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", j.Key)
	response, err := c.http.Do(req)
	if err != nil {
		return Outcome{Status: "pending", Code: "provider_transport"}
	}
	defer response.Body.Close()
	if response.StatusCode == 429 || response.StatusCode >= 500 {
		delay := time.Duration(0)
		if raw := response.Header.Get("Retry-After"); raw != "" {
			if seconds, err := strconv.ParseInt(raw, 10, 32); err == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			} else if at, err := http.ParseTime(raw); err == nil {
				delay = max(0, time.Until(at))
			}
		}
		return Outcome{Status: "pending", Code: "provider_retryable", RetryAfter: delay}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Outcome{Status: "dead", Code: "provider_rejected"}
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	var receipt struct {
		MessageID string `json:"message_id"`
	}
	if err != nil || len(encoded) > 4096 || json.Unmarshal(encoded, &receipt) != nil || !validMessageID(receipt.MessageID) {
		return Outcome{Status: "pending", Code: "provider_protocol"}
	}
	return Outcome{Status: "sent", MessageID: receipt.MessageID}
}

func validMessageID(s string) bool {
	if !strings.HasPrefix(s, "mock-") || len(s) != 69 {
		return false
	}
	for _, r := range s[5:] {
		if !(r >= 'a' && r <= 'f') && !(r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
