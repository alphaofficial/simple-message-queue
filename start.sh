#!/bin/bash
# Start the SQS server with Go workspace disabled
env GOWORK=off go run server.go