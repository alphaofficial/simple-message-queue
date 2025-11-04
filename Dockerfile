# Use Go 1.23 bullseye as base image
FROM golang:1.23-bullseye AS base

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -o simple-message-queue main.go

EXPOSE 8080

CMD ["/build/simple-message-queue"]