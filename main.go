package main

import (
	"fmt"
	"github.com/throttled/throttled/v2"
	"github.com/throttled/throttled/v2/store/memstore"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type GatewayItem struct {
	Frontend     string `yaml:"frontend"`
	Backend      string `yaml:"backend"`
	MaxReqPerSec int    `yaml:"reqsPerSec"`
	MaxBurst     int    `yaml:"burst"`
}

type Configuration struct {
	Routes  []GatewayItem `yaml:"routes"`
	Metrics bool          `yaml:"metrics"`
	Port    string        `yaml:"port"`
}

func RPHandler(backend string, frontend string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequest(r.Method, backend, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for k, v := range r.Header {
			req.Header[k] = v
		}

		client := &http.Client{}

		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)

		if _, err := io.Copy(w, resp.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func LoadGateway(mux *http.ServeMux, store *memstore.MemStore, items []GatewayItem) {
	for _, i := range items {
		quota := throttled.RateQuota{MaxRate: throttled.PerSec(i.MaxReqPerSec), MaxBurst: i.MaxBurst}
		rateLimiter, err := throttled.NewGCRARateLimiter(store, quota)
		if err != nil {
			log.Fatal(err)
		}

		httpRateLimiter := throttled.HTTPRateLimiter{
			RateLimiter: rateLimiter,
			VaryBy:      &throttled.VaryBy{Path: true},
		}

		mux.Handle(i.Frontend, httpRateLimiter.RateLimit(http.HandlerFunc(RPHandler(i.Backend, i.Frontend))))
	}
}

func main() {
	var config Configuration

	data, err := os.ReadFile("rockhopper.yaml")
	if err != nil {
		log.Fatal("readfile err", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatal("unmarshal err", err)
	}

	mux := http.NewServeMux()

	store, err := memstore.New(65536)
	if err != nil {
		log.Fatal(err)
	}

	LoadGateway(mux, store, config.Routes)

	srv := &http.Server{
		Handler:      mux,
		Addr:         fmt.Sprintf("127.0.0.1:%s", config.Port),
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	fmt.Printf("🐧 ice-flow-limiter service is running http://127.0.0.1:%s\n", config.Port)
	fmt.Println("Loaded routes :")
	for _, i := range config.Routes {
		fmt.Printf("http://127.0.0.1:%s%s => %s - ratelimit: %v - burst: %v\n", config.Port, i.Frontend, i.Backend, i.MaxReqPerSec, i.MaxBurst)
	}
	log.Fatal(srv.ListenAndServe())
}
