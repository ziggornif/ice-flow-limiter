package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/requestid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/throttled/throttled/v2"
	"github.com/throttled/throttled/v2/store/memstore"
	"gopkg.in/yaml.v3"
)

type GatewayItem struct {
	Frontend     string `yaml:"frontend"`
	Backend      string `yaml:"backend"`
	MaxReqPerSec int    `yaml:"reqsPerSec"`
	MaxBurst     int    `yaml:"burst"`
	Label        string `yaml:"label"`
}

type IpConfiguration struct {
	Blacklist []string `yaml:"blacklist"`
	Whitelist []string `yaml:"whitelist"`
}

type Configuration struct {
	Routes  []GatewayItem   `yaml:"routes"`
	Metrics bool            `yaml:"metrics"`
	Port    string          `yaml:"port"`
	Ip      IpConfiguration `yaml:"ip"`
}

type ResponseTime struct {
	responseTimeHistogram *prometheus.HistogramVec
}

func (resp *ResponseTime) Collect(method string, route string, code string, responseTime float64) {
	resp.responseTimeHistogram.With(prometheus.Labels{
		"method": method,
		"route":  route,
		"code":   code,
	}).Observe(responseTime)
}

func NewResponseTime(label string) *ResponseTime {
	responseTimeHistogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    fmt.Sprintf("%s_http_request_duration_ms", label),
		Help:    fmt.Sprintf("Duration of HTTP requests received by the %s endpoint in ms", label),
		Buckets: []float64{.1, 5, 15, 50, 100, 200, 300, 400, 500, 1000},
	}, []string{"method", "route", "code"})
	prometheus.MustRegister(responseTimeHistogram)
	return &ResponseTime{
		responseTimeHistogram,
	}
}

func ValidateConfig(config Configuration) error {
	if len(config.Ip.Blacklist) > 0 && len(config.Ip.Whitelist) > 0 {
		return fmt.Errorf("ip whitelisting and blacklisting cannot be used at the same time")
	}

	return nil
}

func isIPBlacklisted(ip string, ipConfig IpConfiguration) bool {
	for _, blacklistedIP := range ipConfig.Blacklist {
		if blacklistedIP == ip {
			return true
		}
	}
	return false
}

func isIPWhitelisted(ip string, ipConfig IpConfiguration) bool {
	for _, blacklistedIP := range ipConfig.Whitelist {
		if blacklistedIP == ip {
			return true
		}
	}
	return false
}

func RPHandler(label string, backend string, requestTotalCounter prometheus.Counter, responseTimeCollector *ResponseTime, ipConfig IpConfiguration) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := requestid.Get(r)
		logrus.WithFields(logrus.Fields{
			"label":      label,
			"method":     r.Method,
			"uri":        r.RequestURI,
			"user-agent": r.UserAgent(),
			"requestid":  id,
		}).Info("Incoming call")

		start := time.Now()
		if requestTotalCounter != nil {
			requestTotalCounter.Inc()
		}

		ip := strings.Split(r.RemoteAddr, ":")[0]
		fmt.Println(r.RemoteAddr)
		if (len(ipConfig.Blacklist) > 0 && isIPBlacklisted(ip, ipConfig)) || (len(ipConfig.Whitelist) > 0 && !isIPWhitelisted(ip, ipConfig)) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			execTime := time.Since(start)
			logrus.WithFields(logrus.Fields{
				"label":          label,
				"method":         r.Method,
				"uri":            r.RequestURI,
				"user-agent":     r.UserAgent(),
				"requestid":      id,
				"execution-time": execTime,
				"ip":             ip,
			}).Errorf("Unauthorized IP %v", ip)
			if responseTimeCollector != nil {
				responseTimeCollector.Collect(r.Method, r.RequestURI, strconv.Itoa(http.StatusForbidden), float64(execTime.Milliseconds()))
			}
			return
		}

		req, err := http.NewRequest(r.Method, backend, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			execTime := time.Since(start)
			logrus.WithFields(logrus.Fields{
				"label":          label,
				"method":         r.Method,
				"uri":            r.RequestURI,
				"user-agent":     r.UserAgent(),
				"requestid":      id,
				"execution-time": execTime,
			}).Errorf("Execution error %v", err.Error())
			if responseTimeCollector != nil {
				responseTimeCollector.Collect(r.Method, r.RequestURI, strconv.Itoa(http.StatusInternalServerError), float64(execTime.Milliseconds()))
			}
			return
		}

		for k, v := range r.Header {
			req.Header[k] = v
		}

		client := &http.Client{}

		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			execTime := time.Since(start)
			logrus.WithFields(logrus.Fields{
				"label":          label,
				"method":         r.Method,
				"uri":            r.RequestURI,
				"user-agent":     r.UserAgent(),
				"requestid":      id,
				"execution-time": execTime,
			}).Errorf("Execution error %v", err.Error())
			if responseTimeCollector != nil {
				responseTimeCollector.Collect(r.Method, r.RequestURI, strconv.Itoa(http.StatusInternalServerError), float64(execTime.Milliseconds()))
			}
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)

		if _, err := io.Copy(w, resp.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			execTime := time.Since(start)
			logrus.WithFields(logrus.Fields{
				"label":          label,
				"method":         r.Method,
				"uri":            r.RequestURI,
				"user-agent":     r.UserAgent(),
				"requestid":      id,
				"execution-time": execTime,
			}).Errorf("Execution error %v", err.Error())
			if responseTimeCollector != nil {
				responseTimeCollector.Collect(r.Method, r.RequestURI, strconv.Itoa(http.StatusInternalServerError), float64(execTime.Milliseconds()))
			}
		}

		execTime := time.Since(start)
		logrus.WithFields(logrus.Fields{
			"label":      label,
			"method":     r.Method,
			"uri":        r.RequestURI,
			"user-agent": r.UserAgent(),
			"requestid":  id,
		}).Infof("Execution time %v", execTime)
		if responseTimeCollector != nil {
			responseTimeCollector.Collect(r.Method, r.RequestURI, strconv.Itoa(http.StatusOK), float64(execTime.Milliseconds()))
		}
	}
}

func DeniedHandler(requestDeniedCounter prometheus.Counter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestDeniedCounter.Inc()
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
	})
}

func LoadGateway(mux *http.ServeMux, store *memstore.MemStore, items []GatewayItem, metrics bool, ipConfig IpConfiguration) {
	for _, i := range items {
		var requestTotalCounter prometheus.Counter
		var requestDeniedCounter prometheus.Counter
		var responseTimeCollector *ResponseTime

		if metrics {
			re, err := regexp.Compile(`\W`)
			if err != nil {
				log.Fatal(err)
			}
			transformed := re.ReplaceAllString(i.Label, "")
			metricLabel := strings.ToLower(transformed)

			requestTotalCounter = prometheus.NewCounter(prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_requests_total", metricLabel),
				Help: fmt.Sprintf("The total number of requests received by the %s endpoint.", metricLabel),
			})
			prometheus.MustRegister(requestTotalCounter)

			requestDeniedCounter = prometheus.NewCounter(prometheus.CounterOpts{
				Name: fmt.Sprintf("%s_requests_denied", metricLabel),
				Help: fmt.Sprintf("The total number of denied requests received by the %s endpoint.", metricLabel),
			})
			prometheus.MustRegister(requestDeniedCounter)

			responseTimeCollector = NewResponseTime(metricLabel)
		}

		if i.MaxReqPerSec == 0 {
			mux.Handle(i.Frontend, http.HandlerFunc(RPHandler(i.Label, i.Backend, requestTotalCounter, responseTimeCollector, ipConfig)))
		} else {
			quota := throttled.RateQuota{MaxRate: throttled.PerSec(i.MaxReqPerSec), MaxBurst: i.MaxBurst}
			rateLimiter, err := throttled.NewGCRARateLimiter(store, quota)
			if err != nil {
				log.Fatal(err)
			}

			httpRateLimiter := throttled.HTTPRateLimiter{
				RateLimiter:   rateLimiter,
				VaryBy:        &throttled.VaryBy{Path: true},
				DeniedHandler: DeniedHandler(requestDeniedCounter),
			}

			mux.Handle(i.Frontend, httpRateLimiter.RateLimit(http.HandlerFunc(RPHandler(i.Label, i.Backend, requestTotalCounter, responseTimeCollector, ipConfig))))
		}
	}
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})

	var config Configuration

	data, err := os.ReadFile("rockhopper.yaml")
	if err != nil {
		log.Fatal("readfile err", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatal("unmarshal err", err)
	}

	err = ValidateConfig(config)
	if err != nil {
		log.Fatal("validation err", err)
	}

	mux := http.NewServeMux()

	store, err := memstore.New(65536)
	if err != nil {
		log.Fatal(err)
	}

	LoadGateway(mux, store, config.Routes, config.Metrics, config.Ip)

	if config.Metrics {
		mux.Handle("/metrics", promhttp.Handler())
	}

	srv := &http.Server{
		Handler:      requestid.Handler(mux),
		Addr:         fmt.Sprintf(":%s", config.Port),
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
