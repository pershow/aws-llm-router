package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aws-cursor-router/internal/openai"
	"github.com/google/uuid"
)

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(data)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) Flush() {
	flusher, ok := r.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	flusher.Flush()
}

func loggingMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Printf("%s %s %d %s", r.Method, r.URL.Path, recorder.status, time.Since(startedAt).String())
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "failed to write response", http.StatusInternalServerError)
	}
}

func writeAdminError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(message)})
}

func writeOpenAIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, openai.ErrorResponse{Error: openai.OpenAIErrorPayload{
		Message: strings.TrimSpace(message),
		Type:    "invalid_request_error",
		Code:    strconv.Itoa(status),
	}})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, maxBodyBytes int64, dst any) error {
	if maxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func extractBearerToken(headerValue string) string {
	headerValue = strings.TrimSpace(headerValue)
	if headerValue == "" {
		return ""
	}

	const prefix = "Bearer "
	if len(headerValue) <= len(prefix) || !strings.EqualFold(headerValue[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(headerValue[len(prefix):])
}

func extractAdminToken(r *http.Request) string {
	if token := extractBearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	return strings.TrimSpace(r.Header.Get("x-salessavvy-token"))
}

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminToken := a.getAdminToken()
		if adminToken == "" {
			writeAdminError(w, http.StatusServiceUnavailable, "salessavvy token is not initialized")
			return
		}

		token := extractAdminToken(r)
		if subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
			writeAdminError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	}
}

func newRequestID() string {
	return "req-" + uuid.NewString()
}

func truncateRunes(value string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	return string(runes[:maxChars])
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeSSEData(w http.ResponseWriter, payload any) error {
	blob, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// 调试日志：打印发送的 SSE 数据
	fmt.Printf("[DEBUG SSE] data: %s\n", string(blob))
	if _, err := io.WriteString(w, "data: "); err != nil {
		return err
	}
	if _, err := w.Write(blob); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n\n"); err != nil {
		return err
	}
	if err := http.NewResponseController(w).Flush(); err != nil {
		return fmt.Errorf("streaming not supported")
	}
	return nil
}

func writeSSEDone(w http.ResponseWriter) error {
	if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func parseLimit(raw string, fallback, maxAllowed int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	if maxAllowed > 0 && value > maxAllowed {
		return maxAllowed
	}
	return value
}

func parseDate(raw string, fallback time.Time) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback.UTC().Format("2006-01-02"), nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return "", err
	}
	return parsed.Format("2006-01-02"), nil
}
