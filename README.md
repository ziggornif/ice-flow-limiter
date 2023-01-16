# ice-flow-limiter

**🔧 WORK IN PROGRESS 🔧**

This project is a simple reverse proxy server that offers rate limiting for each route.

## Configuration

```yml
routes:
  - frontend: "/tweets"
    backend: "http://localhost:8888/tweets"
    label: "tweets"
    reqsPerSec: 10
    burst: 5
  - frontend: "/signin"
    backend: "http://localhost:8888/signin"
    label: "signin"
    reqsPerSec: 1
    burst: 0
metrics: false
port: 8000
ip:
  whitelist|blacklist: # cannot be used at the same time
  - 192.168.86.70
```

## Run

```shell
./ice-flow-limiter
```

Output :
```shell
🐧 ice-flow-limiter service is running http://127.0.0.1:8000
Loaded routes :
http://127.0.0.1:8000/tweets => http://localhost:8888/tweets - ratelimit: 10 - burst: 5
http://127.0.0.1:8000/signin => http://localhost:8888/signin - ratelimit: 1 - burst: 0
```
## IP filtering access

### Whitelist

IP whitelist config allows you to define IP list of authorized callers.

Requests coming from IPs not present in the table will be rejected.

```yaml
ip:
  whitelist:
  - x.x.x.x
  - y.y.y.y
```

### Blacklist

IP blacklist config allows you to define blocked IP list.

Requests coming from IPs present in this table will be rejected.

```yaml
ip:
  blacklist:
  - x.x.x.x
  - y.y.y.y
```

## Metrics

Metrics could be enabled with the `metrics: true | false` parameter.

When metrics are enabled, new metrics are created for each configured route.

### Request counter

The total of all requests on the route.

Example:
```
# HELP tweets_requests_total The total number of requests received by the tweets endpoint.
# TYPE tweets_requests_total counter
tweets_requests_total 1
```

### Request duration

The duration of HTTP requests on the route.

Example :
```
# HELP tweets_http_request_duration_ms Duration of HTTP requests received by the tweets endpoint in ms
# TYPE tweets_http_request_duration_ms histogram
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="0.1"} 0
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="5"} 0
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="15"} 1
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="50"} 1
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="100"} 1
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="200"} 1
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="300"} 1
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="400"} 1
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="500"} 1
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="1000"} 1
tweets_http_request_duration_ms_bucket{code="200",method="GET",route="/tweets",le="+Inf"} 1
tweets_http_request_duration_ms_sum{code="200",method="GET",route="/tweets"} 11
tweets_http_request_duration_ms_count{code="200",method="GET",route="/tweets"} 1
```

## TODO
- [x] routes without rate limit
- [x] IP blacklisting
- [ ] routes path / query params
- [x] query params filters
- [x] headers filters
- [ ] circuit breaker
- [ ] redis / inmem rate limiter modes
