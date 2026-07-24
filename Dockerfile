# ─── Build stage ───────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ynab-bot ./cmd/bot

# ─── Run stage ────────────────────────────────────────────
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Europe/Kyiv

COPY --from=builder /ynab-bot /usr/local/bin/ynab-bot

USER nobody
CMD ["ynab-bot"]
