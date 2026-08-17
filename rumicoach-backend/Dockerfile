# ---- Build Stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Download dependencies first (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/main.go

# ---- Runtime Stage ----
FROM alpine:3.21

WORKDIR /app

# Install CA certificates for HTTPS/OAuth calls, and ffmpeg for transcoding
# Gemini TTS PCM into OGG/Opus WhatsApp voice notes (companion channel).
RUN apk --no-cache add ca-certificates tzdata ffmpeg

COPY --from=builder /app/server .
COPY --from=builder /app/api/openapi.yaml ./api/openapi.yaml

EXPOSE 8080

CMD ["./server"]
