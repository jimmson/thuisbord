# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.25-alpine AS builder
WORKDIR /app

# Module files first for layer caching. (No deps yet, but keeps the pattern.)
COPY go.mod ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 -> fully static binary. -s -w strips debug info.
# tzdata is embedded via `import _ "time/tzdata"`, so no zoneinfo needed in the image.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /thuisbord ./cmd/server

# ---- Final stage ----
# distroless/static ships CA certs + a nonroot user; no shell, no package manager.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /thuisbord /thuisbord
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/thuisbord"]
