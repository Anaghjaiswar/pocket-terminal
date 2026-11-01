# syntax=docker/dockerfile:1

# Stage 1: Build single self-contained PocketTerminal binary
FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY go.mod ./
# Download module dependencies
RUN go mod download || true

COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/pocket-terminal .

# Stage 2: Minimal runtime image with bash, git, and curl
FROM alpine:3.20
RUN apk add --no-cache bash git curl ca-certificates tzdata

WORKDIR /workspace
COPY --from=builder /out/pocket-terminal /usr/local/bin/pocket-terminal

EXPOSE 8080
ENV SHELL=/bin/bash

ENTRYPOINT ["pocket-terminal"]
CMD ["-port", "8080", "-shell", "/bin/bash"]
