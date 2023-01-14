# ice-flow-limiter

This project is a simple reverse proxy server that offers rate limiting for each route.

## Configuration

```yml
routes:
  - frontend: "/tweets"
    backend: "http://localhost:8888/tweets"
    reqsPerSec: 10
    burst: 5
  - frontend: "/signin"
    backend: "http://localhost:8888/signin"
    reqsPerSec: 1
    burst: 0
metrics: false
port: 8000
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