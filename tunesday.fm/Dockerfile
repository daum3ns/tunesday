FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY . .

RUN go mod download
RUN CGO_ENABLED=0 go build -o server ./tunesday.fm/cmd/server

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /build/server .

EXPOSE 8080

VOLUME ["/data"]

ENV TUNESDAY_FM_DATA_DIR=/data

CMD ["./server"]
