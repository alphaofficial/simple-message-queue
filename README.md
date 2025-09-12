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
# Clone and build
git clone <repository>
cd sqs-producer
go build -o sqs-server main.go

# Start with default SQLite storage
./sqs-server

# Or with custom configuration
PORT=9000 DB_PATH=/tmp/sqs.db ./sqs-server
```

The server starts on `http://localhost:8080` with both:
- **Web Console**: `http://localhost:8080/`
- **AWS SQS API**: `http://localhost:8080/` (POST endpoints)

### 2. Use the Web Console

1. **Open your browser** to `http://localhost:8080`
2. **Create queues** using the "Create Queue" button
3. **Send messages** in the "Send Message" tab with JSON payloads
4. **Poll and view messages** with syntax highlighting in the "Messages" tab
5. **Monitor in real-time** with auto-refresh and live metrics

### 3. Test with AWS SDK

```bash
# Run the included test client
go run examples/test-client.go
```

## 📱 Web Console Interface

The CloudUI-style console provides a complete SQS management experience:

### **Dashboard View**
- **Real-time metrics** - queue counts, message stats, throughput
- **Service status** indicators with live updates
- **Quick actions** for common operations

### **Queue Management**
- **Visual queue browser** with creation timestamps and URLs
- **One-click polling** to switch to message view
- **Queue deletion** with confirmation prompts

### **Message Operations**
- **JSON message composer** with syntax validation
- **Message polling** with configurable batch sizes
- **Syntax-highlighted message viewer** with expand/collapse
- **Individual message deletion** with receipt handle tracking

### **Live Features**
- **Auto-refresh toggle** (5-second intervals)
- **Real-time message counts** and queue stats
- **Interactive message expansion** for detailed viewing
- **Responsive design** for desktop and mobile

## 🛠 AWS SDK Integration

### Python Example
```python
import boto3

sqs = boto3.client('sqs',
    endpoint_url='http://localhost:8080',  # Base server URL
    region_name='us-east-1',
    aws_access_key_id='dummy',
    aws_secret_access_key='dummy'
)

# Use existing queue URL (create queues via web console)
queue_url = 'http://localhost:8080/my-queue'

# Send message using queue URL
sqs.send_message(
    QueueUrl=queue_url,
    MessageBody='{"event": "order_created", "orderId": "12345", "timestamp": "2024-01-01T00:00:00Z"}'
)

# Receive messages
messages = sqs.receive_message(QueueUrl=queue_url, MaxNumberOfMessages=10, WaitTimeSeconds=5)
```

### Node.js Example (AWS SDK v3)
```javascript
import { SendMessageCommand, SQSClient, ReceiveMessageCommand } from "@aws-sdk/client-sqs";

// Configure client for local development
const client = new SQSClient({
    endpoint: 'http://localhost:8080',
    region: 'us-east-1',
    credentials: {
        accessKeyId: 'dummy',
        secretAccessKey: 'dummy'
    }
});

const SQS_QUEUE_URL = "http://localhost:8080/orders-queue";

export const sendMessage = async (sqsQueueUrl = SQS_QUEUE_URL) => {
    const command = new SendMessageCommand({
        QueueUrl: sqsQueueUrl,
        DelaySeconds: 10,
        MessageAttributes: {
            EventType: {
                DataType: "String",
                StringValue: "order_created",
            },
            Priority: {
                DataType: "String", 
                StringValue: "high",
            },
            RetryCount: {
                DataType: "Number",
                StringValue: "0",
            },
        },
        MessageBody: JSON.stringify({
            orderId: "12345",
            customerId: "cust_abc123",
            amount: 99.99,
            timestamp: new Date().toISOString()
        })
    });

    const response = await client.send(command);
    return response;
};

export const receiveMessages = async (sqsQueueUrl = SQS_QUEUE_URL) => {
    const command = new ReceiveMessageCommand({
        QueueUrl: sqsQueueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 5,
        MessageAttributeNames: ["All"]
    });

    const response = await client.send(command);
    return response.Messages || [];
};
```

## 🔧 Configuration

### Environment Variables
- `PORT`: Server port (default: 8080)
- `DB_PATH`: SQLite database path (default: ./sqs.db)
- `BASE_URL`: Server base URL (default: http://localhost:PORT)
- `STORAGE_TYPE`: Storage backend (default: sqlite)

### Queue URL Structure
- **Base URL**: The server endpoint (e.g., `http://localhost:8080`)
- **Queue URLs**: Automatically generated as `{BASE_URL}/{QueueName}`
- **Example**: Queue "my-queue" → URL "http://localhost:8080/my-queue"

## 📡 API Endpoints

### Web Console REST API
- `GET /` - CloudUI-style dashboard interface
- `GET /api/status` - Server and queue statistics
- `GET /api/queues` - List all queues with metadata
- `POST /api/queues` - Create new queue
- `DELETE /api/queues/{name}` - Delete specific queue
- `POST /api/messages/poll` - Poll messages from queue
- `POST /api/messages/send` - Send message to queue
- `DELETE /api/messages/delete` - Delete specific message

### AWS SQS Compatible API
- `POST /` - All standard SQS actions via XML API
  - CreateQueue, DeleteQueue, ListQueues
  - SendMessage, ReceiveMessage, DeleteMessage
  - GetQueueAttributes, SetQueueAttributes
  - And more...

## 🏗 Project Structure

```
sqs-producer/
├── main.go                 # Main server entry point
├── src/                    # Source code (Node.js style)
│   ├── api/                # HTTP handlers and routing
│   │   └── handlers.go     # SQS API and Web console handlers
│   ├── storage/            # Storage layer
│   │   ├── storage.go      # Storage interface
│   │   └── sqlite/         # SQLite implementation
│   │       └── sqlite.go
│   └── templates/          # Web console templates
│       └── dashboard.html  # CloudUI-style dashboard
├── examples/               # Test clients and examples
│   ├── test-client.go      # AWS SDK test client
│   └── simple-test.go      # Basic HTTP client
├── build.sh               # Build script
├── start.sh               # Start script
└── go.mod                 # Go module definition
```

## 🎯 Supported SQS Operations

### Queue Operations
✅ CreateQueue - Create named queues
✅ DeleteQueue - Remove queues and all messages
✅ ListQueues - Enumerate all queues with filters
✅ GetQueueUrl - Retrieve queue URL by name
✅ GetQueueAttributes - Query queue configuration
✅ SetQueueAttributes - Update queue settings
✅ PurgeQueue - Clear all messages from queue

### Message Operations
✅ SendMessage - Add single message to queue
✅ SendMessageBatch - Add multiple messages efficiently
✅ ReceiveMessage - Poll messages with visibility timeout
✅ DeleteMessage - Remove message using receipt handle
✅ DeleteMessageBatch - Remove multiple messages
✅ ChangeMessageVisibility - Extend/reduce visibility timeout

### Advanced Features
✅ **Dead Letter Queues** - Automatic retry and DLQ routing
✅ **Message Visibility** - Timeout-based message locking
✅ **Batch Operations** - Efficient multi-message handling
✅ **Queue Attributes** - Configurable timeouts, retention, delays
✅ **Real-time Monitoring** - Live stats and message tracking

## 🧪 Development & Testing

### Build and Run
```bash
# Build binary
go build -o sqs-server main.go

# Start server
./sqs-server

# Or use helper scripts
./build.sh  # Builds the server
./start.sh  # Starts the server
```

### Testing
```bash
# Start server in background
./sqs-server &

# Test with AWS SDK client
go run examples/test-client.go

# Test with simple HTTP client
go run examples/simple-test.go

# Open web console
open http://localhost:8080

# Kill server
pkill sqs-server
```

### Queue Configuration Example
```bash
# Create DLQ first
aws sqs create-queue --queue-name my-dlq --endpoint-url http://localhost:8080

# Create main queue with redrive policy
aws sqs create-queue --queue-name my-queue \
  --attributes MaxReceiveCount=3,RedrivePolicy='{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:123456789012:my-dlq","maxReceiveCount":3}' \
  --endpoint-url http://localhost:8080
```

## 📊 Queue Attributes

| Attribute | Default | Description |
|-----------|---------|-------------|
| `VisibilityTimeout` | 30 seconds | Message lock duration during processing |
| `MaxReceiveCount` | 0 (disabled) | Max receives before DLQ routing |
| `DelaySeconds` | 0 | Delay before message becomes available |
| `MessageRetentionPeriod` | 14 days | How long messages are retained |
| `RedrivePolicy` | none | JSON policy for DLQ configuration |

## 🚀 Why This SQS Server?

### **For Development**
- **No AWS credentials needed** - perfect for local development
- **Instant setup** - one binary, no configuration
- **Visual debugging** - see messages in real-time with the web console
- **Fast iteration** - modify, test, repeat without AWS latency

### **For Testing**
- **Deterministic behavior** - consistent local environment
- **Easy cleanup** - delete SQLite file to reset state
- **Full AWS compatibility** - same code works in production
- **Message inspection** - debug payloads with syntax highlighting

### **For Production**
- **Self-hosted** - no vendor lock-in or usage charges
- **Pluggable storage** - SQLite for simple, PostgreSQL for scale
- **Standard protocols** - works with all AWS SDK libraries
- **Monitoring built-in** - web console for operations

## 📄 License

MIT License - see LICENSE file for details

---

**Built with ❤️ for developers who want SQS without the complexity**