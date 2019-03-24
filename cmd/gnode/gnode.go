package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/globbie/gnode/pkg/knowdy"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

var (
	listenAddress string
	shardConfig   string
	requestsMax   int
	duration      time.Duration
)

func init() {
	var configPath string

	flag.StringVar(&listenAddress, "listen-address", "localhost:8082", "gnode listen address")
	flag.StringVar(&configPath, "config-path", "/etc/knowdy/shard.gsl", "path to knowdy config")
	flag.IntVar(&requestsMax, "requests-limit", 10, "maximum number of requests are processed simultaneously")
	flag.DurationVar(&duration, "request-limit-duration", 1*time.Second, "free slot awaiting time")
	flag.Parse()

	shardConfigBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalln("could not read shard config, error:", err)
	}
	shardConfig = string(shardConfigBytes)
}

func main() {
	shard, err := knowdy.New(shardConfig)
	if err != nil {
		log.Fatalln("could not create knowdy shard, error:", err)
	}
	defer shard.Del()

	router := http.NewServeMux()
	router.Handle("/gsl", measurer(limiter(gslHandler(shard), requestsMax, duration)))
	router.Handle("/metrics", metricsHandler)

	server := &http.Server{
		Handler:      logger(router),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  15 * time.Second,
		Addr:         listenAddress,
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		log.Println("shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			log.Fatalln("could not gracefully shutdown the server:", server.Addr)
		}
		close(done)
	}()

	log.Println("server is ready to handle requests at:", listenAddress)

	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("could not listen on %s, err: %v\n", server.Addr, err)
	}

	<-done
	log.Println("server stopped.")
}

func logger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Method, r.URL.Path, r.URL.Query(), r.RemoteAddr, r.UserAgent())
		h.ServeHTTP(w, r)
	})
}

func limiter(h http.Handler, requestsMax int, duration time.Duration) http.Handler {
	semaphore := make(chan struct{}, requestsMax)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }()
			h.ServeHTTP(w, r)
		case <-time.After(duration):
			http.Error(w, "server is busy", http.StatusTooManyRequests)
			log.Println("no free slots")
			return
		}
	})
}

func gslHandler(shard *knowdy.Shard) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		result, taskType, err := shard.RunTask(string(body))
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, result)

		if metrics, ok := r.Context().Value(metricsKey).(*Metrics); ok {
			metrics.Success = true
			metrics.TaskType = taskType
		}
	})
}
