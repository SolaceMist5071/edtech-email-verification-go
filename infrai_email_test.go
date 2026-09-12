package main

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

func TestSendVerificationRetriesRateLimitAndReadsEnvelope(t *testing.T) {
	attempts := 0
	client := newInfraiEmailClient("test-key")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if req.Method != http.MethodPost {
			t.Fatalf("method=%s, want POST", req.Method)
		}
		if req.Header.Get("Authorization") != "Bearer test-key" || req.Header.Get("Idempotency-Key") != "signup-42" {
			t.Fatal("required request headers missing")
		}
		if attempts == 1 {
			return response(http.StatusTooManyRequests, `{"ok":false,"data":null,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`, "0"), nil
		}
		return response(http.StatusOK, `{"ok":true,"data":{"message_id":"msg_42"},"error":null,"metadata":{}}`, ""), nil
	})}
	client.sleep = func(context.Context, time.Duration) error { return nil }

	messageID, err := client.SendVerification(context.Background(), verificationEmail{To: "learner@example.edu", Subject: "Verify", HTML: "<p>Verify</p>"}, "signup-42")
	if err != nil {
		t.Fatal(err)
	}
	if messageID != "msg_42" || attempts != 2 {
		t.Fatalf("messageID=%q attempts=%d", messageID, attempts)
	}
}

func response(status int, body, retryAfter string) *http.Response {
	header := make(http.Header)
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}
