package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"os"
	"strings"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	publicBaseURL := strings.TrimRight(envOr("PUBLIC_BASE_URL", "http://localhost:8080"), "/")
	service := newSignupService(newInfraiEmailClient(apiKey), publicBaseURL)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /signup", signupHandler(service))
	mux.HandleFunc("GET /verify", verifyHandler(service))
	mux.HandleFunc("GET /educator/report", reportHandler(service))

	address := envOr("LISTEN_ADDR", ":8080")
	log.Printf("edtech verification service listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func signupHandler(service *signupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input signupRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if _, err := mail.ParseAddress(input.LearnerEmail); err != nil || input.CourseID == "" || input.CourseTitle == "" || input.EducatorID == "" || input.Deadline.IsZero() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "learner_email, course_id, course_title, educator_id, and deadline are required"})
			return
		}
		record, err := service.Signup(r.Context(), input)
		if err != nil {
			var apiErr *infraiError
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Error()})
				return
			}
			log.Printf("send verification email: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "verification email could not be sent"})
			return
		}
		writeJSON(w, http.StatusAccepted, record)
	}
}

func verifyHandler(service *signupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record, ok := service.Verify(r.URL.Query().Get("token"))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "verification token not found"})
			return
		}
		writeJSON(w, http.StatusOK, record)
	}
}

func reportHandler(service *signupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		educatorID := r.URL.Query().Get("educator_id")
		if educatorID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "educator_id is required"})
			return
		}
		writeJSON(w, http.StatusOK, service.Report(educatorID))
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
