FROM golang:1.23-bookworm AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o client ./cmd/client/main.go

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /build/client /app/client
COPY --from=builder /build/cert.pem /app/cert.pem
COPY --from=builder /build/key.pem /app/key.pem

EXPOSE 8080

CMD ["/app/client"]