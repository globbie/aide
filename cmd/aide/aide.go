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
	"github.com/globbie/aide/pkg/knowdy"
	"github.com/globbie/aide/pkg/mail"
	"github.com/globbie/aide/pkg/session"
)

type Config struct {
	ListenAddress     string        `json:"listen-address"`
	KnowdyAddress     string        `json:"knowdy-address"`
	KnowdyServiceName string        `json:"knowdy-service-name"`
	KnowdyShards      []string      `json:"knowdy-shards"`
	LingProcAddress   string        `json:"ling-service-name"`
	KndConfigPath     string        `json:"shard-config"`
	MailServerAddress string        `json:"mail-server-address"`
	MailServerUser    string        `json:"mail-server-user"`
	MailServerAuth    string        `json:"mail-server-auth"`
	RequestsMax       int           `json:"requests-max"`
	SlotAwaitDuration time.Duration `json:"slot-await-duration"`
	VerifyKeyPath     string        `json:"verify-key-path"`
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
		lingAddress   string
		requestsMax   int
		duration      time.Duration
	)

	flag.StringVar(&configPath, "config-path", "/etc/aide/aide.json", "path to AIDE config")
	flag.StringVar(&kndConfigPath, "knd-config-path", "/etc/aide/shard.gsl", "path to Knowdy config")
	flag.StringVar(&listenAddress, "listen-address", "", "AIDE listen address")
	flag.StringVar(&lingAddress, "ling-address", "", "Glottie ling proc address")
	flag.IntVar(&requestsMax, "requests-limit", 10, "maximum number of requests to process simultaneously")
	flag.DurationVar(&duration, "request-limit-duration", 1*time.Second, "free slot awaiting time")
	flag.Parse()

	{ // load config
		configData, err := ioutil.ReadFile(configPath)
		if err != nil {
			log.Fatalln("could not read AIDE config, error:", err)
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
		if lingAddress != "" {
			cfg.LingProcAddress = lingAddress
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
	shard, err := knowdy.New(KndConfig, cfg.KnowdyAddress, cfg.KnowdyServiceName, cfg.LingProcAddress,
		cfg.KnowdyShards, runtime.GOMAXPROCS(0))
	if err != nil {
		log.Fatalln("could not create a Knowdy Shard, error:", err)
	}
	defer shard.Del()

	ms, e := mail.New(cfg.MailServerAddress, cfg.MailServerUser, cfg.MailServerAuth)
	if e != nil {
		log.Fatalln("failed to create mail service, error:", e)
	}

	router := http.NewServeMux()
	router.Handle("/session", measurer(limiter(sessionHandler(shard),
		cfg.RequestsMax, cfg.SlotAwaitDuration)))
	router.Handle("/gsl", measurer(limiter(gslHandler(shard),
		cfg.RequestsMax, cfg.SlotAwaitDuration)))
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

	hostname, _ := os.Hostname()
	currentTime := time.Now()

	log.Println("AIDE server is ready to handle requests at ", hostname, " ",
		cfg.ListenAddress, " Glottie service:", cfg.LingProcAddress,
		" shards:", cfg.KnowdyShards)

	var msg = "AIDE server started at " + currentTime.String()
	log.Println(msg, ms)
	//go ms.SendMail(from, to, msg)

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
		claims := token.Claims.(jwt.MapClaims)
		log.Printf("Token for user %s expires %v", claims["email"], claims["exp"])
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
		// TODO output formats
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
			http.Error(w, "URL format error: " + err.Error(), http.StatusBadRequest)
			return
		}

		msg := new(knowdy.Message)
		if err := schema.NewDecoder().Decode(msg, r.Form); err != nil {
			http.Error(w, "URL error: " + err.Error(), http.StatusBadRequest)
			return
		}
		result, _, err := shard.ProcessMsg(*msg)
		if err != nil {
			http.Error(w, "internal server error: " + err.Error(), http.StatusInternalServerError)
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

func sessionHandler(shard *knowdy.Shard) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_, err := r.Cookie("SID")
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "{\"status\"\"SID is already set\":}")
			return
		}

		ses, _ := session.New(r)
		result, cookies, err := shard.CreateChatSession(ses)
		if err != nil {
			http.Error(w, "failed to open a session: " + err.Error(), http.StatusInternalServerError)
			return
		}

		for _, cookie := range cookies {
			http.SetCookie(w, &cookie)
			log.Println("++ set cookie:", cookie.Name)
		}
		
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, result)
	})
}
