package main

import (
	"context"
	"testing"
	"time"
)

type recordingEmailSender struct {
	messageID string
	email     verificationEmail
	key       string
}

func (s *recordingEmailSender) SendVerification(_ context.Context, email verificationEmail, key string) (string, error) {
	s.email = email
	s.key = key
	return s.messageID, nil
}

func TestEducatorReportDeadlineDecision(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		deadline    time.Time
		verify      bool
		wantPending int
		wantOverdue int
	}{
		{name: "future unverified learner is pending", deadline: now.Add(24 * time.Hour), wantPending: 1},
		{name: "past unverified learner is overdue", deadline: now.Add(-time.Minute), wantPending: 1, wantOverdue: 1},
		{name: "verified learner is never overdue", deadline: now.Add(-time.Hour), verify: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &recordingEmailSender{messageID: "msg_123"}
			service := newSignupService(sender, "http://localhost:8080")
			service.now = func() time.Time { return now }
			service.newToken = func() (string, error) { return "fixed-token", nil }
			record, err := service.Signup(context.Background(), signupRequest{
				LearnerEmail: "learner@example.edu", CourseID: "go-101", CourseTitle: "Go APIs",
				EducatorID: "teacher-7", Deadline: tt.deadline,
			})
			if err != nil {
				t.Fatal(err)
			}
			if tt.verify {
				service.Verify(record.Token)
			}

			report := service.Report("teacher-7")
			if len(report) != 1 {
				t.Fatalf("got %d report rows, want 1", len(report))
			}
			if report[0].Pending != tt.wantPending || report[0].OverdueUnverified != tt.wantOverdue {
				t.Fatalf("pending=%d overdue=%d, want pending=%d overdue=%d", report[0].Pending, report[0].OverdueUnverified, tt.wantPending, tt.wantOverdue)
			}
			if sender.key != "signup:go-101\x00learner@example.edu" {
				t.Fatalf("unexpected idempotency key %q", sender.key)
			}
		})
	}
}
