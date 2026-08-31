package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ashleyfullero/scrapeowl/internal/scheduler"
	"github.com/ashleyfullero/scrapeowl/internal/store"
)

// uptime tracks when the server started
var serverStart = time.Now()

// Server is the main HTTP server
type Server struct {
	http     *http.Server
	hub      *Hub
	handlers *Handlers
	logger   *slog.Logger
}

// NewServer creates and configures the HTTP server
func NewServer(addr string, st *store.Store, sched *scheduler.Scheduler, logger *slog.Logger) *Server {
	hub := NewHub(logger)
	handlers := NewHandlers(st, hub, sched, logger)
	rl := NewRateLimiter(200) // 200 requests/min per IP

	mux := http.NewServeMux()

	// Static dashboard
	mux.Handle("/", http.FileServer(http.Dir("web")))

	// Health check (unauthenticated, no rate limit)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":    "ok",
			"version":   "dev",
			"uptime_s":  int(time.Since(serverStart).Seconds()),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// WebSocket
	mux.HandleFunc("/ws", hub.ServeWS)

	// API routes with CORS + logging + rate limit middleware
	api := withCORS(withLogging(logger, withRateLimit(rl, mux)))

	// Jobs
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.ListJobs(w, r)
		case http.MethodPost:
			handlers.CreateJob(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case isRunAction(path):
			if r.Method == http.MethodPost {
				handlers.RunJob(w, r)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case isEnableAction(path):
			if r.Method == http.MethodPost {
				handlers.EnableJob(w, r)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case isDisableAction(path):
			if r.Method == http.MethodPost {
				handlers.DisableJob(w, r)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			switch r.Method {
			case http.MethodGet:
				handlers.GetJob(w, r)
			case http.MethodPut:
				handlers.UpdateJob(w, r)
			case http.MethodDelete:
				handlers.DeleteJob(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}
	})

	// Runs
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.ListRuns(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/runs/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case isStopAction(path) && r.Method == http.MethodPost:
			handlers.StopRun(w, r)
		case isDataAction(path) && r.Method == http.MethodGet:
			handlers.GetRunData(w, r)
		case r.Method == http.MethodGet:
			handlers.GetRun(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Stats & Utility
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		handlers.GetStats(w, r)
	})
	mux.HandleFunc("/api/proxy/stats", func(w http.ResponseWriter, r *http.Request) {
		handlers.GetProxyStats(w, r)
	})
	mux.HandleFunc("/api/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handlers.ValidateYAML(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	return &Server{
		http: &http.Server{
			Addr:         addr,
			Handler:      api,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		hub:      hub,
		handlers: handlers,
		logger:   logger,
	}
}

// Start begins listening for connections
func (s *Server) Start() error {
	s.logger.Info("ScrapeOwl dashboard", "url", fmt.Sprintf("http://%s", s.http.Addr))
	return s.http.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// Hub returns the WebSocket hub for external event publishing
func (s *Server) Hub() *Hub {
	return s.hub
}

// --- Middleware ---

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		logger.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration", time.Since(start),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func isRunAction(path string) bool {
	return len(path) > 4 && path[len(path)-4:] == "/run"
}

func isStopAction(path string) bool {
	return len(path) > 5 && path[len(path)-5:] == "/stop"
}

func isDataAction(path string) bool {
	return len(path) > 5 && path[len(path)-5:] == "/data"
}

func isEnableAction(path string) bool {
	return len(path) > 7 && path[len(path)-7:] == "/enable"
}

func isDisableAction(path string) bool {
	return len(path) > 8 && path[len(path)-8:] == "/disable"
}
