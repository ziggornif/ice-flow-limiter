package main

import (
	"fmt"
	"github.com/throttled/throttled/v2"
	"github.com/throttled/throttled/v2/store/memstore"
	"io"
	"log"
	"net/http"
	"time"
)

type GatewayItem struct {
	Frontend     string
	Backend      string
	MaxReqPerSec int
	MaxBurst     int
}

func RPHandler(backend string, frontend string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("FRONT", frontend)
		fmt.Println("Request URI", r.RequestURI)
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
	fmt.Println("Hello penguins ! 🐧")

	mux := http.NewServeMux()

	store, err := memstore.New(65536)
	if err != nil {
		log.Fatal(err)
	}

	items := []GatewayItem{
		{
			Frontend:     "/tweets",
			Backend:      "http://localhost:8888/tweets",
			MaxReqPerSec: 10,
			MaxBurst:     5,
		},
		{
			Frontend:     "/signin",
			Backend:      "http://localhost:8888/signin",
			MaxReqPerSec: 1,
			MaxBurst:     0,
		},
	}

	LoadGateway(mux, store, items)

	srv := &http.Server{
		Handler:      mux,
		Addr:         "127.0.0.1:8000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
