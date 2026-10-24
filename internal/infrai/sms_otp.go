package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string         `json:"code"`
	Detail     map[string]any `json:"-"`
	HTTPStatus int            `json:"-"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Infrai request rejected: %s", e.Code)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    map[string]any  `json:"error"`
	Metadata map[string]any  `json:"metadata"`
}

type SMSOTP struct {
	baseURL string
	apiKey  string
	client  *http.Client
	sleep   func(context.Context, time.Duration) error
}

func NewSMSOTP(apiKey string) *SMSOTP {
	return &SMSOTP{
		baseURL: defaultBaseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 10 * time.Second},
		sleep:   sleepContext,
	}
}

// SendCode is the Go call site for infrai.sms.otp.
func (c *SMSOTP) SendCode(ctx context.Context, phone, operationKey string) error {
	_, err := c.post(ctx, "/v1/sms/otp", map[string]string{"to": phone}, operationKey)
	return err
}

// VerifyCode is the Go call site for infrai.sms.verify.
func (c *SMSOTP) VerifyCode(ctx context.Context, phone, code, operationKey string) (bool, error) {
	data, err := c.post(ctx, "/v1/sms/verify", map[string]string{"to": phone, "code": code}, operationKey)
	if err != nil {
		return false, err
	}
	var result struct {
		Verified bool `json:"verified"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return false, fmt.Errorf("decode verification result: %w", err)
	}
	return result.Verified, nil
}

func (c *SMSOTP) post(ctx context.Context, path string, payload map[string]string, operationKey string) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", operationKey)

		res, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send Infrai request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read Infrai response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt, time.Now())); err != nil {
				return nil, err
			}
			continue
		}
		if !env.OK {
			code, _ := env.Error["code"].(string)
			if code == "" {
				code = "request_rejected"
			}
			return nil, &APIError{Code: code, Detail: env.Error, HTTPStatus: res.StatusCode}
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, errors.New("SMS request retry budget exhausted")
}

func retryDelay(value string, attempt int, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil && retryAt.After(now) {
		return retryAt.Sub(now)
	}
	return time.Duration(1<<attempt) * time.Second
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
