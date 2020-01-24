package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"log"

	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	prometheus.MustRegister(requestsCounter)
}

var version string = os.Getenv("VERSION")

var listenAddr = flag.String("listen address",  ":8080", "The address to listen on for web requests")
var checkAddr  = flag.String("check address",   ":8090", "The address to listen on for live and ready checks.")
var metricAddr = flag.String("metrics address", ":9090", "The address to listen on for metric pulls.")

var requestsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "A counter for received requests",
		ConstLabels: map[string]string{
			"version": version,
		},
	},
	[]string{"code", "method"})

func serveHttp(s *http.Server) {
	log.Printf("Server started at %s", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Printf("Starting server failed")
	}
}

func serveChecks(addr string, health healthcheck.Handler) {
	log.Printf("Serving checks on port %s", addr)
	if err := http.ListenAndServe(addr, health); err != nil {
		log.Printf("Starting checks listener failed")
	}
}

func serveMetrics(addr string) {
	log.Printf("Serving metrics on port %s", addr)
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Printf("Starting Prometheus listener failed")
	}
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(w, "Error getting hostname\n")
		return
	}
	fmt.Fprintf(w, "Host: %s, Version: %s\n", hostname, version)
}

func promRequestHandler(handler http.Handler) http.Handler {
	return promhttp.InstrumentHandlerCounter(requestsCounter, handler)
}

func main() {

	// flags
	flag.Parse()

	// logs
	log.Printf("Web App Version: %s\n", version)

	// graceful server shutdown
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	// health checks
	healthz := healthcheck.NewHandler()

	// mux
	mux := http.NewServeMux()
	mux.HandleFunc("/", httpHandler)

	srv := &http.Server{
		Addr: *listenAddr,
		Handler: promRequestHandler(mux),
	}

	go serveHttp(srv)
	go serveChecks(*checkAddr, healthz)
	go serveMetrics(*metricAddr)

	<-quit
	log.Println("Shutting down gracefully the server ...")
	// Gracefully server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

