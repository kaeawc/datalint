# Stage 1: build the Go binary. CGO is required by the tree-sitter
# Python grammar, so install build-base for gcc/musl-dev.
FROM golang:1.26-alpine AS build
RUN apk add --no-cache build-base
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN go build -ldflags "-s -w" -o /out/datalint ./cmd/datalint

# Stage 2: minimal runtime. Alpine matches the build's musl libc.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/datalint /usr/local/bin/datalint
ENTRYPOINT ["/usr/local/bin/datalint"]
