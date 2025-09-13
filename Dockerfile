# Use Go 1.21 bullseye as base image
FROM golang:1.21-bullseye AS base

# Move to working directory /build
WORKDIR /build

# Copy the go.mod and go.sum files first for better caching
COPY go.mod go.sum ./

# Install dependencies
RUN go mod download

# Copy the entire source code into the container
COPY . .

# Build the application (CGO needed for SQLite)
RUN CGO_ENABLED=1 go build -o sqs-bridge main.go

# Document the port that may need to be published
EXPOSE 8080

# Start the application
CMD ["/build/sqs-bridge"]