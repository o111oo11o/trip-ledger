FROM golang:1.25.4-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /trip-ledger ./cmd/bot

FROM alpine:3.19
RUN apk add --no-cache ca-certificates sqlite
COPY --from=builder /trip-ledger /usr/local/bin/trip-ledger

VOLUME /data
ENV DB_PATH=/data/trip-ledger.db

ENTRYPOINT ["trip-ledger"]
