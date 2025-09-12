# SQS-Compatible Server

A Go implementation of an AWS SQS-compatible server with pluggable storage backends (SQLite, PostgreSQL).

## Features

- **AWS SQS API Compatibility**: Works with existing AWS SDK clients
- **Pluggable Storage**: SQLite and PostgreSQL support
- **Dead Letter Queue**: Automatic message handling for failed processing
- **Message Visibility**: Timeout management for message processing
- **Standard SQS Operations**: CreateQueue, SendMessage, ReceiveMessage, DeleteMessage, etc.

## Quick Start

### 1. Start the Server

```bash
# Using default SQLite storage
go run cmd/server/main.go

# Or with custom configuration
PORT=9000 DB_PATH=/tmp/sqs.db go run cmd/server/main.go
```

The server will start on `http://localhost:8080` by default.

### 2. Test with AWS SDK Client

```bash
# Run the test client
go run cmd/test-client/main.go
```

### 3. Use with Any AWS SDK

```python
# Python example
import boto3

sqs = boto3.client('sqs',
    endpoint_url='http://localhost:8080',
    region_name='us-east-1',
    aws_access_key_id='dummy',
    aws_secret_access_key='dummy'
)

# Create queue
response = sqs.create_queue(QueueName='my-queue')
queue_url = response['QueueUrl']

# Send message
sqs.send_message(
    QueueUrl=queue_url,
    MessageBody='Hello World'
)

# Receive messages
messages = sqs.receive_message(QueueUrl=queue_url)
```

```javascript
// Node.js example
const AWS = require('aws-sdk');

const sqs = new AWS.SQS({
    endpoint: 'http://localhost:8080',
    region: 'us-east-1',
    accessKeyId: 'dummy',
    secretAccessKey: 'dummy'
});

// Create and use queue
const params = { QueueName: 'my-queue' };
sqs.createQueue(params).promise()
    .then(data => console.log('Queue created:', data.QueueUrl));
```

## Configuration

Environment variables:

- `PORT`: Server port (default: 8080)
- `DB_PATH`: SQLite database file path (default: ./sqs.db)
- `BASE_URL`: Base URL for queue URLs (default: http://localhost:PORT)
- `STORAGE_TYPE`: Storage backend type (default: sqlite)

## Supported SQS APIs

### Queue Operations
- CreateQueue
- DeleteQueue
- ListQueues
- GetQueueUrl
- GetQueueAttributes
- SetQueueAttributes
- PurgeQueue

### Message Operations
- SendMessage
- SendMessageBatch
- ReceiveMessage
- DeleteMessage
- DeleteMessageBatch
- ChangeMessageVisibility

### Dead Letter Queue Features
- Automatic DLQ handling based on MaxReceiveCount
- RedrivePolicy configuration
- Background processing for expired messages

## Queue Attributes

Supported queue attributes:
- `VisibilityTimeout`: Message visibility timeout (default: 30 seconds)
- `MaxReceiveCount`: Max receives before moving to DLQ (default: 0 = disabled)
- `DelaySeconds`: Message delivery delay (default: 0)
- `MessageRetentionPeriod`: How long to keep messages (default: 14 days)

- `RedrivePolicy`: JSON policy for DLQ configuration

Example with DLQ:
```bash
# Create DLQ first
aws sqs create-queue --queue-name my-dlq --endpoint-url http://localhost:8080

# Create main queue with redrive policy
aws sqs create-queue --queue-name my-queue \
  --attributes MaxReceiveCount=3,RedrivePolicy='{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:123456789012:my-dlq","maxReceiveCount":3}' \
  --endpoint-url http://localhost:8080
```

## Development

### Project Structure
```
├── cmd/
│   ├── server/          # Main server application
│   └── test-client/     # AWS SDK test client
├── internal/
│   ├── api/            # SQS API handlers
│   ├── storage/        # Storage interfaces and implementations
│   │   ├── interface/  # Storage interface definition
│   │   ├── sqlite/     # SQLite implementation
│   │   └── postgres/   # PostgreSQL implementation (TODO)
│   └── config/         # Configuration management
└── migrations/         # Database migrations
```

### Adding New Storage Backend

1. Implement the `storage.Storage` interface
2. Add initialization in `cmd/server/main.go`
3. Add configuration options

### Running Tests

```bash
# Start server in background
go run cmd/server/main.go &

# Run test client
go run cmd/test-client/main.go

# Kill server
pkill -f "go run cmd/server/main.go"
```

## License

MIT License


go build -o sqs-server ./cmd/server

lsof -ti :8080 | xargs kill -9