FROM golang:1.26.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG SERVICE=api

RUN CGO_ENABLED=0 GOOS=linux go build \
    -o /app/bin/service \
    ./cmd/${SERVICE}


FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/bin/service /app/service

COPY --from=builder /app/migrations /app/migrations

RUN addgroup -S app && adduser -S app -G app
USER app

EXPOSE 8080

CMD ["/app/service"]