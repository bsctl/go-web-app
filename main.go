package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	prometheus.MustRegister(requestsCounter)
}

var version string = os.Getenv("VERSION")

var listenAddr = flag.String("listen", ":8080", "The address to listen on for web requests")
var metricAddr = flag.String("metric", ":9090", "The address to listen on for metric pulls.")

var requestsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "A counter for received requests",
		ConstLabels: map[string]string{
			"version": version,
		},
	},
	[]string{"code", "method"})

func serveHTTP(s *http.Server) {
	log.Printf("Server started at %s", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Printf("Starting server failed")
	}
}

func serveMetrics(addr string) {
	log.Printf("Serving metrics on port %s", addr)
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Printf("Starting Prometheus listener failed")
	}
}

func httpHandler(w http.ResponseWriter, req *http.Request) {
	var hostname, remoteAddress string
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		fmt.Fprintf(w, "Error getting hostname\n")
		return
	}
	remoteAddress, _, err = net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		fmt.Fprintf(w, "Error getting remote address\n")
		return
	}
	fmt.Fprintf(w,
		"Server name: %s\nServer version: %s\nRemote client address: %s\n",
		hostname, version, remoteAddress)
}

func loadHandler(w http.ResponseWriter, req *http.Request) {
	ncpu := runtime.NumCPU()
	runtime.GOMAXPROCS(ncpu)
	quit := make(chan bool)
	for i := 0; i < ncpu; i++ {
		go func() {
			for {
				select {
				case <-quit:
					return
				default:
				}
			}
		}()
	}

	time.Sleep(10 * time.Second)
	for i := 0; i < ncpu; i++ {
		fmt.Fprintf(w, "Loading CPU: %d\n", i)
		quit <- true
	}
}

func probeHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "ok")
}

func metricsHandler(handler http.Handler) http.Handler {
	return promhttp.InstrumentHandlerCounter(requestsCounter, handler)
}

func delayHandler(w http.ResponseWriter, req *http.Request) {
	time.Sleep(10 * time.Second)
	fmt.Fprintf(w, "ok")
}

func main() {

	// flags
	flag.Parse()

	// logs
	log.Printf("Web App Version: %s\n", version)

	// graceful server shutdown
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	// mux
	mux := http.NewServeMux()
	mux.HandleFunc("/", httpHandler)
	mux.HandleFunc("/load", loadHandler)
	mux.HandleFunc("/ready", probeHandler)
	mux.HandleFunc("/live", probeHandler)
	mux.HandleFunc("/delay", delayHandler)

	srv := &http.Server{
		Addr:    *listenAddr,
		Handler: metricsHandler(mux),
	}

	go serveHTTP(srv)
	go serveMetrics(*metricAddr)

	<-quit
	log.Println("Shutting down gracefully the server ...")
	// Gracefully server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
