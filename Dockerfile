FROM golang:1.22.5-alpine3.20 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . . 
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-w -s" -o /wave

FROM alpine:3.20 AS prod
WORKDIR /app
COPY --from=builder /wave .
CMD [ "./wave" ]
