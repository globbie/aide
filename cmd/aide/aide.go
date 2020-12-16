package main

import (
	"bytes"
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
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"golang.org/x/text/language"

	"github.com/dgrijalva/jwt-go"
	"github.com/dgrijalva/jwt-go/request"
	"github.com/gorilla/schema"
        "github.com/gorilla/mux"

	"github.com/globbie/aide/pkg/knowdy"
	"github.com/globbie/aide/pkg/mail"
	"github.com/globbie/aide/pkg/session"
)

type Config struct {
	ListenAddress     string        `json:"listen-address"`
	ServiceDomain     string        `json:"service-domain"`
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
	SignKeyPath       string        `json:"sign-key-path"`
	StaticPath        string        `json:"static-path"`
	VerifyKeyPath     string        `json:"verify-key-path"`
}

var (
	cfg       *Config
	KndConfig string
	VerifyKey *rsa.PublicKey
	SignKey   *rsa.PrivateKey
)

type spaHandler struct {
	staticPath string
	indexPath  string
}

// todo(n.rodionov): write a separate function for each {} excess block
func init() {
	var (
		configPath    string
		kndConfigPath string
		listenAddress string
		lingAddress   string
		requestsMax   int
		staticPath    string
		duration      time.Duration
	)

	flag.StringVar(&configPath,    "config-path", "/etc/aide/aide.json", "path to AIDE config")
	flag.StringVar(&kndConfigPath, "knd-config-path", "/etc/aide/shard.gsl", "path to Knowdy config")
	flag.StringVar(&listenAddress, "listen-address", "", "AIDE listen address")
	flag.StringVar(&staticPath,    "static-path", "", "path to static content")
	flag.StringVar(&lingAddress,   "ling-address", "", "Glottie ling proc address")
	flag.IntVar(&requestsMax,      "requests-limit", 10, "max number of requests to process simultaneously")
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
		if staticPath != "" {
			cfg.StaticPath = staticPath
		}
		if listenAddress != "" {
			cfg.ListenAddress = listenAddress
		}
		if lingAddress != "" {
			cfg.LingProcAddress = lingAddress
		}
	}

	{ // load shard config
		shardConfigBytes, err := ioutil.ReadFile(cfg.KndConfigPath)
		if err != nil {
			log.Fatalln("could not read shard config, error:", err)
		}
		KndConfig = string(shardConfigBytes)
	}

	{ // load sign key
		signKeyBytes, err := ioutil.ReadFile(cfg.SignKeyPath)
		if err != nil {
			log.Fatalln("failed to read sign key:", err)
		}
		SignKey, err = jwt.ParseRSAPrivateKeyFromPEM(signKeyBytes)
		if err != nil {
			log.Fatalln("failed to parse sign key:", err)
		}
	}
	{ // load verify key
		verifyBytes, err := ioutil.ReadFile(cfg.VerifyKeyPath)
		if err != nil {
			log.Fatalf("could not read verify key file('%v'), error: %v", cfg.VerifyKeyPath, err)
		}
		VerifyKey, err = jwt.ParseRSAPublicKeyFromPEM(verifyBytes)
		if err != nil {
			log.Fatalln("failed to parse verify key:", err)
		}
	}

	if duration != 0 {
		cfg.SlotAwaitDuration = duration
	}
	if requestsMax != 0 {
		cfg.RequestsMax = requestsMax
	}
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the absolute path to prevent directory traversal
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		// if we failed to get the absolute path respond with a 400 bad request
		// and stop
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// prepend the path with the path to the static directory
	path = filepath.Join(h.staticPath, path)

	// check whether a file exists at the given path
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		// log.Println("dynamic resource:" + path)

		// file does not exist, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) stating the
		// file, return a 500 internal server error and stop
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// otherwise, use http.FileServer to serve the static dir
	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

func main() {
	shard, err := knowdy.New(KndConfig, cfg.KnowdyAddress, cfg.KnowdyServiceName, cfg.LingProcAddress,
		cfg.ServiceDomain, cfg.KnowdyShards, runtime.GOMAXPROCS(0))
	if err != nil {
		log.Fatalln("could not create a Knowdy Shard, error:", err)
	}
	defer shard.Del()

	ms, e := mail.New(cfg.MailServerAddress, cfg.MailServerUser, cfg.MailServerAuth)
	if e != nil {
		log.Fatalln("failed to create mail service, error:", e)
	}

	router := mux.NewRouter()
	router.Handle("/session", measurer(limiter(sessionHandler(shard),
		cfg.RequestsMax, cfg.SlotAwaitDuration)))
	router.Handle("/query", measurer(limiter(queryHandler(shard),
		cfg.RequestsMax, cfg.SlotAwaitDuration)))
	router.Handle("/gsl", authorization(measurer(limiter(gslHandler(shard),
		cfg.RequestsMax, cfg.SlotAwaitDuration))))
	router.Handle("/msg", authorization(measurer(limiter(msgHandler(shard), cfg.RequestsMax, cfg.SlotAwaitDuration))))
	router.Handle("/metrics", metricsHandler)

	spa := spaHandler{staticPath: cfg.StaticPath, indexPath: "index.html"}
	router.PathPrefix("/").Handler(spa)

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
			log.Fatalln("failed to gracefully shutdown the server:", server.Addr)
		}
		close(done)
	}()

	hostname, _ := os.Hostname()
	currentTime := time.Now()

	log.Println("AIDE server is ready to handle requests at ", hostname, " ",
		cfg.ListenAddress, " Glottie service:", cfg.LingProcAddress,
		" shards:", cfg.KnowdyShards, " static path:", cfg.StaticPath)

	var msg = "AIDE server started at " + currentTime.String()
	log.Println(msg, ms)
	//go ms.SendMail(from, to, msg)

	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("failed to listen on %s, err: %v\n", server.Addr, err)
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
			log.Println(err.Error());
			http.Error(w, "unauthorized " + err.Error(), http.StatusUnauthorized)
			return
		}
		claims := token.Claims.(jwt.MapClaims)
		ses, _ := session.New(r)

		log.Printf("== UserId: %s, ShardId: %s Token expires: %s  Langs:%s",
			claims["uid"], claims["shard"],
			time.Unix(int64(claims["exp"].(float64)),0), ses.Langs)

		ses.UserId = claims["uid"].(string)
		ses.ShardId = claims["shard"].(string)
		
		ctx := context.WithValue(r.Context(), "session", ses)
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
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseForm(); err != nil {
			http.Error(w, "{\"error\":\"" + err.Error() + "\"}", http.StatusBadRequest)
			return
		}
		msg := new(knowdy.Message)
		if err := schema.NewDecoder().Decode(msg, r.Form); err != nil {
			http.Error(w, "URL error: " + err.Error(), http.StatusBadRequest)
			return
		}
		if ses, ok := r.Context().Value("session").(*session.ChatSession); ok {
			msg.ChatSession = ses
		}
		result, err := shard.ProcessMsg(msg)
		if err != nil {
			http.Error(w, "internal server error: " + err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, result)

		//if metrics, ok := r.Context().Value(metricsKey).(*Metrics); ok {
		//	metrics.Success = true
		//	metrics.TaskType = taskType
		//}
	})
}

func queryHandler(shard *knowdy.Shard) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		gsl, ok := r.URL.Query()["gsl"]
		if !ok || len(gsl) < 1 {
			http.Error(w, "{\"error\":\"URL param gsl is missing\"}", http.StatusBadRequest)
			return
		}
		lang := "en"
		var Langs []language.Tag
		Langs, _, _ = language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
		if len(Langs) > 0 {
			lang = Langs[0].String()
			i := strings.Index(lang, "-")
			if i != -1 {
				lang = lang[:i]
			}
		}
		buf := bytes.Buffer{}
		// TODO graph expand depth option
		buf.WriteString("{task{format JSON}")
		buf.WriteString("{locale " + lang + "}")
		buf.WriteString("{repo ~")
		buf.WriteString(gsl[0])
		buf.WriteString("}}")

		result, _, err := shard.RunTask(buf.String(), len(buf.String()))
		if err != nil {
			log.Println(result)
			// TODO set error status
			http.Error(w, "{\"error\":\"" + result + "\"}", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, result)
	})
}
	
func sessionHandler(shard *knowdy.Shard) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		{  // check SID cookie
			cookie, err := r.Cookie("sid")
			if err == nil {
				log.Println(">> sid cookie already set: " + cookie.Value)				
				claims := jwt.MapClaims{}
				_, e := jwt.ParseWithClaims(cookie.Value, &claims, func(token *jwt.Token) (interface{}, error) {
					return VerifyKey, nil
				})
				if e != nil {
					http.Error(w, "invalid SID", http.StatusBadRequest)
					return
				}
				for key, val := range claims {
					fmt.Printf("Key: %v, value: %v\n", key, val)
				}
				_, _ = io.WriteString(w, "{\"sid\":\"" + cookie.Value + "\"}")
				return
			}
		}
		ses, _ := session.New(r)
		result, cookies, err := shard.CreateChatSession(ses, SignKey)
		if err != nil {
			http.Error(w, "failed to open a session: " + err.Error(), http.StatusInternalServerError)
			return
		}
		for _, cookie := range cookies {
			http.SetCookie(w, cookie)
		}
		_, _ = io.WriteString(w, result)
	})
}
