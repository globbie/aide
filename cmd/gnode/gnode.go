package main

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/dgrijalva/jwt-go/request"

	"github.com/globbie/gnode/pkg/knowdy"
)

type Config struct {
	ListenAddress   string `json:"listen-address"`
	ShardConfigPath string `json:"shard-config"`

	RequestsMax       int           `json:"requests-max"`
	SlotAwaitDuration time.Duration `json:"slot-await-duration"`

	VerifyKeyPath string `json:"verify-key-path"`
}

var (
	cfg         *Config
	shardConfig string
	VerifyKey   *rsa.PublicKey
)

// todo(n.rodionov): write a separate function for each {} excess block
func init() {
	var (
		configPath      string
		shardConfigPath string
		listenAddress   string
		verifyKeyPath   string
	)

	flag.StringVar(&configPath, "config-path", "/etc/gnode/config.json", "path to the config file")
	flag.StringVar(&shardConfigPath, "shard-config-path", "", "redefine shard config path")
	flag.StringVar(&listenAddress, "listen-address", "", "redefine listen address")
	flag.StringVar(&verifyKeyPath, "verify-key-path", "", "redefine verify key path")
	flag.Parse()

	{ // load config
		configData, err := ioutil.ReadFile(configPath)
		if err != nil {
			log.Fatalln("could not read gnode config, error:", err)
		}
		err = json.Unmarshal(configData, &cfg)
		if err != nil {
			log.Fatalln("could not unmarshal config file, error:", err)
		}
	}

	{ // redefine config with cmd-line parameters
		if shardConfigPath != "" {
			cfg.ShardConfigPath = shardConfigPath
		}
		if listenAddress != "" {
			cfg.ListenAddress = listenAddress
		}
		if verifyKeyPath != "" {
			cfg.VerifyKeyPath = verifyKeyPath
		}
	}

	{ // load shard config
		shardConfigBytes, err := ioutil.ReadFile(cfg.ShardConfigPath)
		if err != nil {
			log.Fatalln("could not read shard config, error:", err)
		}
		shardConfig = string(shardConfigBytes)
	}

	{ // load verify key
		verifyBytes, err := ioutil.ReadFile(cfg.VerifyKeyPath)
		if err != nil {
			log.Fatalf("could not read verify key file('%v'), error: %v", cfg.VerifyKeyPath, err)
		}
		VerifyKey, err = jwt.ParseRSAPublicKeyFromPEM(verifyBytes)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

func main() {
	shard, err := knowdy.New(shardConfig)
	if err != nil {
		log.Fatalln("could not create knowdy shard, error:", err)
	}
	defer shard.Del()

	router := http.NewServeMux()
	router.Handle("/gsl", measurer(authorization(limiter(gslHandler(shard), cfg.RequestsMax, cfg.SlotAwaitDuration))))
	router.Handle("/metrics", metricsHandler)

	server := &http.Server{
		Handler:      logger(router),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  15 * time.Second,
		Addr:         cfg.ListenAddress,
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

	log.Println("server is ready to handle requests at:", cfg.ListenAddress)

	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("could not listen on %s, err: %v\n", server.Addr, err)
	}

	<-done
	log.Println("server stopped")
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

func authorization(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := request.ParseFromRequest(r, request.AuthorizationHeaderExtractor, func(token *jwt.Token) (interface{}, error) {
			return VerifyKey, nil
		})
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		log.Println("authorized:", token.Claims)
		h.ServeHTTP(w, r)
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
