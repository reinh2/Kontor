// Package telegram implements the Telegram Bot API webhook channel: verified
// inbound updates with durable deduplication, and a bounded retrying sender.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Sender delivers one outbound message to a chat. Implementations must be
// safe for concurrent use.
type Sender interface {
	Send(ctx context.Context, chatID int64, text string) error
}

// BotAPIConfig configures the real Bot API sender.
type BotAPIConfig struct {
	Token   string
	BaseURL string        // default https://api.telegram.org; tests point this at a fake
	Timeout time.Duration // per-attempt timeout, default 10s
	Client  *http.Client
}

// BotAPISender posts sendMessage calls with bounded retries, exponential
// backoff with jitter, and Retry-After support for 429 responses.
type BotAPISender struct {
	endpoint string
	client   *http.Client
	timeout  time.Duration
}

const senderMaxAttempts = 3

func NewBotAPISender(config BotAPIConfig) (*BotAPISender, error) {
	if config.Token == "" {
		return nil, errors.New("telegram: bot token is required")
	}
	base := strings.TrimRight(config.BaseURL, "/")
	if base == "" {
		base = "https://api.telegram.org"
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	client := config.Client
	if client == nil {
		client = &http.Client{}
	}
	return &BotAPISender{
		endpoint: base + "/bot" + config.Token + "/sendMessage",
		client:   client,
		timeout:  config.Timeout,
	}, nil
}

func (s *BotAPISender) Send(ctx context.Context, chatID int64, text string) error {
	payload, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return fmt.Errorf("telegram: encode sendMessage: %w", err)
	}
	var lastErr error
	for attempt := 1; attempt <= senderMaxAttempts; attempt++ {
		retryAfter, err := s.attempt(ctx, payload)
		if err == nil {
			return nil
		}
		lastErr = err
		var permanent *permanentSendError
		if errors.As(err, &permanent) {
			return err
		}
		if attempt == senderMaxAttempts {
			break
		}
		delay := time.Duration(1<<uint(attempt-1))*500*time.Millisecond +
			time.Duration(rand.Int63n(int64(250*time.Millisecond)))
		if retryAfter > delay {
			delay = retryAfter
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("telegram: send failed after %d attempts: %w", senderMaxAttempts, lastErr)
}

type permanentSendError struct{ status int }

func (e *permanentSendError) Error() string {
	return fmt.Sprintf("telegram: Bot API rejected the message with HTTP %d", e.status)
}

func (s *BotAPISender) attempt(ctx context.Context, payload []byte) (time.Duration, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("telegram: build request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("telegram: transport: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		_ = response.Body.Close()
	}()
	switch {
	case response.StatusCode == http.StatusOK:
		return 0, nil
	case response.StatusCode == http.StatusTooManyRequests:
		var body struct {
			Parameters struct {
				RetryAfter int `json:"retry_after"`
			} `json:"parameters"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4<<10)).Decode(&body)
		retryAfter := time.Duration(body.Parameters.RetryAfter) * time.Second
		if retryAfter > 30*time.Second {
			retryAfter = 30 * time.Second
		}
		return retryAfter, fmt.Errorf("telegram: rate limited (HTTP 429)")
	case response.StatusCode >= 500:
		return 0, fmt.Errorf("telegram: Bot API unavailable (HTTP %d)", response.StatusCode)
	default:
		// 4xx other than 429 will not succeed on retry (bad chat, blocked bot).
		return 0, &permanentSendError{status: response.StatusCode}
	}
}
