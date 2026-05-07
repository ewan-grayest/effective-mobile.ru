package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// Log levels in increasing severity order.
const (
	LevelDebug = 0 // detailed step-by-step diagnostics
	LevelInfo  = 1 // normal API operations
	LevelWarn  = 2 // operations that may modify or destroy data
	LevelError = 3 // failures and exceptions
)

var currentLevel = LevelInfo

// initLogger reads LOG_LEVEL from the environment and sets currentLevel.
// Defaults to INFO when the variable is empty or unrecognised.
func initLogger() {
	level := strings.ToLower(getEnv("LOG_LEVEL", "info"))
	switch level {
	case "debug":
		currentLevel = LevelDebug
	case "info":
		currentLevel = LevelInfo
	case "warn", "warning":
		currentLevel = LevelWarn
	case "error":
		currentLevel = LevelError
	default:
		log.Printf("[WARN] Unknown LOG_LEVEL %q, falling back to 'info'", level)
		currentLevel = LevelInfo
		return
	}
	log.Printf("[INFO] Log level: %s", strings.ToUpper(level))
}

func logDebug(format string, args ...interface{}) {
	if currentLevel <= LevelDebug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func logInfo(format string, args ...interface{}) {
	if currentLevel <= LevelInfo {
		log.Printf("[INFO] "+format, args...)
	}
}

func logWarn(format string, args ...interface{}) {
	if currentLevel <= LevelWarn {
		log.Printf("[WARN] "+format, args...)
	}
}

func logError(format string, args ...interface{}) {
	if currentLevel <= LevelError {
		log.Printf("[ERROR] "+format, args...)
	}
}

// responseRecorder wraps http.ResponseWriter so the middleware can read
// the status code that the handler wrote.
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// withLogging wraps an HTTP handler so that every request gets entry and
// exit log lines. Status >= 500 becomes ERROR, 4xx becomes WARN, otherwise INFO.
func withLogging(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

		logInfo("→ %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		handler(rec, r)

		duration := time.Since(start)
		switch {
		case rec.status >= 500:
			logError("← %s %s %d in %s", r.Method, r.URL.Path, rec.status, duration)
		case rec.status >= 400:
			logWarn("← %s %s %d in %s", r.Method, r.URL.Path, rec.status, duration)
		default:
			logInfo("← %s %s %d in %s", r.Method, r.URL.Path, rec.status, duration)
		}
	}
}
