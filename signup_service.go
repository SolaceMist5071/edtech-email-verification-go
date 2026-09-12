package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"sort"
	"sync"
	"time"
)

type signupRequest struct {
	LearnerEmail string    `json:"learner_email"`
	CourseID     string    `json:"course_id"`
	CourseTitle  string    `json:"course_title"`
	EducatorID   string    `json:"educator_id"`
	Deadline     time.Time `json:"deadline"`
}

type enrollment struct {
	LearnerEmail  string    `json:"learner_email"`
	CourseID      string    `json:"course_id"`
	CourseTitle   string    `json:"course_title"`
	EducatorID    string    `json:"educator_id"`
	Deadline      time.Time `json:"deadline"`
	EmailVerified bool      `json:"email_verified"`
	MessageID     string    `json:"message_id"`
	Token         string    `json:"-"`
}

type educatorCourseReport struct {
	CourseID          string `json:"course_id"`
	CourseTitle       string `json:"course_title"`
	Learners          int    `json:"learners"`
	Verified          int    `json:"verified"`
	Pending           int    `json:"pending"`
	OverdueUnverified int    `json:"overdue_unverified"`
}

type signupService struct {
	mu            sync.RWMutex
	sender        emailSender
	publicBaseURL string
	now           func() time.Time
	newToken      func() (string, error)
	byToken       map[string]*enrollment
	bySignup      map[string]*enrollment
}

func newSignupService(sender emailSender, publicBaseURL string) *signupService {
	return &signupService{
		sender:        sender,
		publicBaseURL: publicBaseURL,
		now:           time.Now,
		newToken:      randomToken,
		byToken:       make(map[string]*enrollment),
		bySignup:      make(map[string]*enrollment),
	}
}

func (s *signupService) Signup(ctx context.Context, in signupRequest) (enrollment, error) {
	key := in.CourseID + "\x00" + in.LearnerEmail
	s.mu.RLock()
	existing := s.bySignup[key]
	s.mu.RUnlock()
	if existing != nil {
		return *existing, nil
	}

	token, err := s.newToken()
	if err != nil {
		return enrollment{}, fmt.Errorf("create verification token: %w", err)
	}
	link := s.publicBaseURL + "/verify?token=" + token
	email := verificationEmail{
		To:      in.LearnerEmail,
		Subject: "Verify your email for " + in.CourseTitle,
		HTML: fmt.Sprintf(
			"<p>Confirm your email to receive <strong>%s</strong>.</p><p><a href=\"%s\">Verify email</a></p><p>Course deadline: %s</p>",
			html.EscapeString(in.CourseTitle), html.EscapeString(link), in.Deadline.UTC().Format(time.RFC3339),
		),
	}
	messageID, err := s.sender.SendVerification(ctx, email, "signup:"+key)
	if err != nil {
		return enrollment{}, err
	}

	record := &enrollment{
		LearnerEmail: in.LearnerEmail, CourseID: in.CourseID, CourseTitle: in.CourseTitle,
		EducatorID: in.EducatorID, Deadline: in.Deadline, MessageID: messageID, Token: token,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.bySignup[key]; existing != nil {
		return *existing, nil
	}
	s.bySignup[key] = record
	s.byToken[token] = record
	return *record, nil
}

func (s *signupService) Verify(token string) (enrollment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.byToken[token]
	if record == nil {
		return enrollment{}, false
	}
	record.EmailVerified = true
	return *record, true
}

func (s *signupService) Report(educatorID string) []educatorCourseReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reports := make(map[string]*educatorCourseReport)
	for _, record := range s.bySignup {
		if record.EducatorID != educatorID {
			continue
		}
		report := reports[record.CourseID]
		if report == nil {
			report = &educatorCourseReport{CourseID: record.CourseID, CourseTitle: record.CourseTitle}
			reports[record.CourseID] = report
		}
		report.Learners++
		if record.EmailVerified {
			report.Verified++
		} else {
			report.Pending++
			if record.Deadline.Before(s.now()) {
				report.OverdueUnverified++
			}
		}
	}
	result := make([]educatorCourseReport, 0, len(reports))
	for _, report := range reports {
		result = append(result, *report)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CourseID < result[j].CourseID })
	return result
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
