# SQS Bridge

A lightweight, SQS-compatible message queue server that supports multiple storage backends including SQLite and PostgreSQL.

### Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd sqs-bridge
```

2. Install dependencies:
```bash
go mod download
```

3. Configure environment variables (optional):
```bash
export PORT=8080
export DB_PATH=./sqs.db
export BASE_URL=http://localhost:8080
```

## Build Instructions

### Development
```bash
go run main.go
```

### Production Build
```bash
go build -o sqs-bridge main.go
./sqs-bridge
```

### Docker (optional)
```dockerfile
FROM golang:1.21.5-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o sqs-bridge main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/sqs-bridge .
EXPOSE 8080
CMD ["./sqs-bridge"]
```

## Example Usage

### TypeScript (AWS SDK v3)

```typescript
import { SQSClient, SendMessageCommand, ReceiveMessageCommand, DeleteMessageCommand } from "@aws-sdk/client-sqs";

// Configure client for local SQS bridge
const sqsClient = new SQSClient({
  region: "us-east-1", // random
  endpoint: "http://localhost:8080",
  credentials: {
    accessKeyId: "fake",
    secretAccessKey: "fake"
  }
});

// Send a message
async function sendMessage() {
  const command = new SendMessageCommand({
    QueueUrl: "http://localhost:8080/queues/my-queue",
    MessageBody: JSON.stringify({
      id: "123",
      data: "Hello World"
    })
  });

  const response = await sqsClient.send(command);
  console.log("Message sent:", response.MessageId);
}

// Receive and process messages
async function receiveMessages() {
  const command = new ReceiveMessageCommand({
    QueueUrl: "http://localhost:8080/queues/my-queue",
    MaxNumberOfMessages: 10,
    WaitTimeSeconds: 20
  });

  const response = await sqsClient.send(command);

  for (const message of response.Messages || []) {
    console.log("Received:", message.Body);

    // Delete message after processing
    await sqsClient.send(new DeleteMessageCommand({
      QueueUrl: "http://localhost:8080/queues/my-queue",
      ReceiptHandle: message.ReceiptHandle
    }));
  }
}
```

## Dashboard

Access the web dashboard at: **http://localhost:8080/dashboard**

The dashboard provides:
- Queue management and monitoring
- Message browsing and inspection

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `PORT` | `8080` | Server port |
| `DB_PATH` | `./sqs.db` | SQLite database path |
| `BASE_URL` | `http://localhost:8080` | Base URL for queue URLs |

## License

MIT