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

## Usage

### Docker

#### SQLite (Default)
```bash
# Quick start with SQLite storage
docker-compose up -d

# View logs
docker-compose logs -f
```

#### PostgreSQL
```bash
# Run with PostgreSQL storage
docker-compose -f docker-compose.postgres.yml up -d

# View logs
docker-compose -f docker-compose.postgres.yml logs -f
```

The server will be available at `http://localhost:8080`

### Local Development

```bash
# Install dependencies
go mod download

# Run with SQLite (default)
go run main.go

# Run with PostgreSQL (requires DATABASE_URL)
STORAGE_ADAPTER=postgres DATABASE_URL="postgres://user:pass@localhost/db" go run main.go
```

### Configuration

Environment variables:
- `STORAGE_ADAPTER`: `sqlite` (default) or `postgres`
- `SQLITE_DB_PATH`: Path to SQLite database file (default: `./sqs.db`)
- `DATABASE_URL`: PostgreSQL connection string (for postgres adapter)
- `PORT`: Server port (default: `8080`)
- `BASE_URL`: Base URL for queue URLs (default: `http://localhost:PORT`)

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

Access the web dashboard at: **http://localhost:8080**

The dashboard provides:
- Queue management and monitoring
- Message browsing and inspection


## License

MIT