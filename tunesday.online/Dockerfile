FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o server ./tunesday.online/cmd/server

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*

# yt-dlp does the YouTube extraction for radio stream mode and title
# lookups - same engine the CLI's mpv path relies on. Pinned via rebuilds:
#   docker compose exec tunesday python3 -m pip install -U yt-dlp
RUN python3 -m pip install --no-cache-dir --break-system-packages yt-dlp

WORKDIR /app

COPY --from=builder /build/server .
COPY scripts/backup.sh /app/scripts/backup.sh
RUN chmod +x /app/scripts/backup.sh

EXPOSE 8080

VOLUME ["/data"]

ENV TUNESDAY_ONLINE_DATA_DIR=/data

CMD ["./server"]
