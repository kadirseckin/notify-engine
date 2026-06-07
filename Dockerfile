FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/migrate ./cmd/migrate

FROM alpine:3.19 AS api
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/api /bin/api
COPY --from=builder /app/docs /app/docs
EXPOSE 8080
ENTRYPOINT ["/bin/api"]

FROM alpine:3.19 AS worker
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/worker /bin/worker
ENTRYPOINT ["/bin/worker"]

FROM alpine:3.19 AS migrate
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/migrate /bin/migrate
COPY migrations /migrations
ENTRYPOINT ["/bin/migrate"]
