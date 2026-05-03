# Stage 1: build the Go binary.
FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /out/datalint ./cmd/datalint

# Stage 2: minimal runtime.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/datalint /usr/local/bin/datalint
ENTRYPOINT ["/usr/local/bin/datalint"]
