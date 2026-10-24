package infrai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSMSRequestBoundary(t *testing.T) {
	tests := []struct {
		name     string
		call     func(*SMSOTP) error
		path     string
		body     string
		response string
	}{
		{
			name: "send code",
			call: func(client *SMSOTP) error {
				return client.SendCode(context.Background(), "+14155550100", "order-42:send")
			},
			path: "/v1/sms/otp", body: `{"to":"+14155550100"}`, response: `{"ok":true,"data":{},"metadata":{}}`,
		},
		{
			name: "verify code",
			call: func(client *SMSOTP) error {
				verified, err := client.VerifyCode(context.Background(), "+14155550100", "123456", "order-42:verify")
				if err == nil && !verified {
					t.Fatal("verification result was false")
				}
				return err
			},
			path: "/v1/sms/verify", body: `{"code":"123456","to":"+14155550100"}`, response: `{"ok":true,"data":{"verified":true},"metadata":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewSMSOTP("test-key")
			client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				raw, _ := io.ReadAll(req.Body)
				if req.Method != http.MethodPost || req.URL.Path != tt.path || string(raw) != tt.body {
					t.Fatalf("request = %s %s %s", req.Method, req.URL.Path, raw)
				}
				if req.Header.Get("Authorization") != "Bearer test-key" || req.Header.Get("Idempotency-Key") == "" {
					t.Fatal("missing request headers")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tt.response))}, nil
			})}
			client.sleep = func(context.Context, time.Duration) error { return nil }
			if err := tt.call(client); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		header  string
		attempt int
		want    time.Duration
	}{
		{name: "seconds", header: "3", want: 3 * time.Second},
		{name: "HTTP date", header: "Fri, 04 Sep 2026 08:00:05 GMT", want: 5 * time.Second},
		{name: "exponential fallback", attempt: 2, want: 4 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryDelay(tt.header, tt.attempt, now); got != tt.want {
				t.Fatalf("retryDelay() = %s, want %s", got, tt.want)
			}
		})
	}
}
