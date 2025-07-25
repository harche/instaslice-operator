package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

// Server represents the metrics HTTP server
type Server struct {
	server *http.Server
	port   string
}

// NewServer creates a new metrics server
func NewServer(port string) *Server {
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		server: server,
		port:   port,
	}
}

// Start starts the metrics server
func (s *Server) Start(ctx context.Context) error {
	klog.Infof("Starting metrics server on port %s", s.port)
	klog.Infof("Metrics available at http://localhost:%s/metrics", s.port)
	klog.Infof("Health check available at http://localhost:%s/health", s.port)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.Errorf("Metrics server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	klog.Infof("Shutting down metrics server")

	// Create a context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("metrics server shutdown error: %w", err)
	}

	klog.Infof("Metrics server stopped")
	return nil
}

// GetPort returns the port the server is configured to use
func (s *Server) GetPort() string {
	return s.port
}
