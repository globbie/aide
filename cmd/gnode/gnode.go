package main

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/dgrijalva/jwt-go/request"
        "github.com/gorilla/schema"

	"github.com/globbie/gnode/pkg/knowdy"
)

type Config struct {
	ListenAddress     string        `json:"listen-address"`
	GltAddress        string        `json:"glt-address"`
	KndConfigPath     string        `json:"shard-config"`
	RequestsMax       int           `json:"requests-max"`
	SlotAwaitDuration time.Duration `json:"slot-await-duration"`

	VerifyKeyPath string `json:"verify-key-path"`
}

type Msg struct {
	Content   string `schema:"t,required"`
	Lang      string `schema:"lang,required"`
	SessionId string `schema:"sid,required"`
}

var (
	cfg       *Config
	KndConfig string
	VerifyKey *rsa.PublicKey
)

// todo(n.rodionov): write a separate function for each {} excess block
func init() {
	var (
		configPath    string
		kndConfigPath string
		verifyKeyPath string
		listenAddress string
		requestsMax   int
		duration      time.Duration
	)

	flag.StringVar(&configPath, "config-path", "/etc/gnode/gnode.json", "path to Gnode config")
	flag.StringVar(&kndConfigPath, "knd-config-path", "/etc/gnode/shard.gsl", "path to Knowdy config")
	flag.StringVar(&listenAddress, "listen-address", "", "Gnode listen address")
	flag.IntVar(&requestsMax, "requests-limit", 10, "maximum number of requests to process simultaneously")
	flag.DurationVar(&duration, "request-limit-duration", 1*time.Second, "free slot awaiting time")
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
		if kndConfigPath != "" {
			cfg.KndConfigPath = kndConfigPath
		}
		if listenAddress != "" {
			cfg.ListenAddress = listenAddress
		}
		if verifyKeyPath != "" {
			cfg.VerifyKeyPath = verifyKeyPath
		}
	}

	{ // load shard config
		shardConfigBytes, err := ioutil.ReadFile(cfg.KndConfigPath)
		if err != nil {
			log.Fatalln("could not read shard config, error:", err)
		}
		KndConfig = string(shardConfigBytes)
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

	if duration != 0 {
		cfg.SlotAwaitDuration = duration
	}
	if requestsMax != 0 {
		cfg.RequestsMax = requestsMax
	}
}

func main() {
	shard, err := knowdy.New(KndConfig, cfg.GltAddress, runtime.GOMAXPROCS(0))
	if err != nil {
		log.Fatalln("could not create kndShard, error:", err)
	}
	defer shard.Del()

	router := http.NewServeMux()
	router.Handle("/gsl", measurer(authorization(limiter(gslHandler(shard),
		cfg.RequestsMax, cfg.SlotAwaitDuration))))
	router.Handle("/msg", measurer(limiter(msgHandler(shard), cfg.RequestsMax, cfg.SlotAwaitDuration)))
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

	log.Println("Gnode server is ready to handle requests at:", cfg.ListenAddress)

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
		//claims := token.Claims.(jwt.MapClaims)
		// log.Printf("Token for user %s expires %v", claims["email"], claims["exp"])
		ctx := context.WithValue(r.Context(), "token", token)
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func gslHandler(shard *knowdy.Shard) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		result, taskType, err := shard.RunTask(string(body), len(body))
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

func msgHandler(shard *knowdy.Shard) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "URL parsing error", http.StatusBadRequest)
			return
		}

		msg := new(Msg)
		if err := schema.NewDecoder().Decode(msg, r.Form); err != nil {
			http.Error(w, "URL error: " + err.Error(), http.StatusBadRequest)
			return
		}

		// r.Context().Value("token").(*jwt.Token)
		result, _, err := shard.ProcessMsg(msg.SessionId, msg.Content, msg.Lang)
		if err != nil {
			log.Println(err.Error())
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// allow connections from web apps
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, result)

		//if metrics, ok := r.Context().Value(metricsKey).(*Metrics); ok {
		//	metrics.Success = true
		//	metrics.TaskType = taskType
		//}
	})
}
