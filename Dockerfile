FROM golang:1.23.0-alpine AS builder

ARG APP

WORKDIR /app

COPY go.mod go.sum .

RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -o wave -a -ldflags="-s -w" -installsuffix cgo ${APP}

FROM alpine:3.20 AS prod

RUN apk add --no-cache yt-dlp

COPY --from=builder /app/wave /app/wave

WORKDIR /app

ENTRYPOINT ["./wave"]

