FROM golang:1.25-bookworm AS builder
WORKDIR /src

RUN apt-get update && apt-get install -y gcc

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/flux_agent .
RUN gcc -O2 -static -o /out/tcp_pool tcp-preconn/tcp_pool.c -lpthread || \
    gcc -O2 -o /out/tcp_pool tcp-preconn/tcp_pool.c -lpthread

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates dsniff tzdata && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/flux_agent /usr/local/bin/flux_agent
COPY --from=builder /out/tcp_pool /usr/local/bin/tcp_pool

ENV TZ=Asia/Shanghai
WORKDIR /etc/flux_agent
ENTRYPOINT ["/usr/local/bin/flux_agent"]
