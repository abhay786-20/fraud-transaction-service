# ---- Build stage: full Go toolchain, discarded after this stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/fraud-transaction-service ./cmd/main.go

# ---- Final stage: just the compiled binary, nothing else ----
FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder --chown=app:app /app/bin/fraud-transaction-service .

USER app
EXPOSE 8082

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8082/health || exit 1

ENTRYPOINT ["./fraud-transaction-service"]
