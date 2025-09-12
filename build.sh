#!/bin/bash
# Build the SQS server with Go workspace disabled
env GOWORK=off go build -o sqs-server server.go
echo "Build complete: sqs-server"