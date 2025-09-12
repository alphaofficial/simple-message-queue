#!/bin/bash
# Build the SQS server
go build -o sqs-server main.go
echo "Build complete: sqs-server"