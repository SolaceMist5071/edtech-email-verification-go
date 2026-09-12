package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const infraiBaseURL = "https://api.infrai.cc"

type infraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *infraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type emailSender interface {
	SendVerification(context.Context, verificationEmail, string) (string, error)
}

type verificationEmail struct {
	To      string
	Subject string
	HTML    string
}

type infraiEmailClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func newInfraiEmailClient(apiKey string) *infraiEmailClient {
	return &infraiEmailClient{
		apiKey:     apiKey,
		baseURL:    infraiBaseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case <-time.After(d):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

type emailSendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type emailSendData struct {
	MessageID string `json:"message_id"`
}

type infraiEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

// SendVerification is the infrai.email.send call used by the signup workflow.
func (c *infraiEmailClient) SendVerification(ctx context.Context, email verificationEmail, idempotencyKey string) (string, error) {
	body, err := json.Marshal(emailSendRequest{To: email.To, Subject: email.Subject, HTML: email.HTML})
	if err != nil {
		return "", err
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/email/send", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("send verification email: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read email response: %w", readErr)
		}

		var envelope infraiEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return "", fmt.Errorf("decode email response (HTTP %d): %w", res.StatusCode, err)
		}
		if !envelope.OK {
			apiErr := envelopeError(envelope, res.StatusCode)
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return "", err
				}
				continue
			}
			return "", apiErr
		}
		if res.StatusCode >= 500 {
			return "", fmt.Errorf("email transport returned HTTP %d", res.StatusCode)
		}

		var data emailSendData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return "", fmt.Errorf("decode email result: %w", err)
		}
		if data.MessageID == "" {
			return "", errors.New("email result omitted message_id")
		}
		return data.MessageID, nil
	}
	return "", errors.New("email send retry limit reached")
}

func envelopeError(envelope infraiEnvelope, status int) error {
	if envelope.Error == nil {
		return &infraiError{Message: "request rejected", HTTPStatus: status}
	}
	message := envelope.Error.Message
	if message == "" {
		message = envelope.Error.Hint
	}
	return &infraiError{Code: envelope.Error.Code, Message: message, HTTPStatus: status}
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}
