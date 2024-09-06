FROM golang:1.23.0-alpine AS builder

ARG APP
ARG UPX_VERSION=4.2.4

RUN apk add --no-cache xz curl && \
  curl -Ls https://github.com/upx/upx/releases/download/v${UPX_VERSION}/upx-${UPX_VERSION}-amd64_linux.tar.xz -o - | tar xvJf - -C /tmp && \
  cp /tmp/upx-${UPX_VERSION}-amd64_linux/upx /usr/local/bin/ && \
  chmod +x /usr/local/bin/upx && \
  apk del xz && \
  rm -rf /var/cache/apk/*

WORKDIR /app

COPY go.mod go.sum .

RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -o wave -a -ldflags="-s -w" -installsuffix cgo ${APP}

RUN upx --ultra-brute -qq wave && upx -t wave

FROM scratch AS prod

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /app/wave /wave

ENTRYPOINT ["/wave"]