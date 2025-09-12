package storage

import (
	"context"
	"time"
)

type Message struct {
	ID                string
	QueueName         string
	Body              string
	Attributes        map[string]string
	MessageAttributes map[string]MessageAttribute
	ReceiptHandle     string
	ReceiveCount      int
	MaxReceiveCount   int
	VisibilityTimeout time.Time
	VisibleAt         *time.Time
	CreatedAt         time.Time
	DelaySeconds      int
	MD5OfBody         string
	MD5OfAttributes   string

	// FIFO-specific fields
	MessageGroupId         string // Required for FIFO queues
	MessageDeduplicationId string // Optional - for deduplication
	SequenceNumber         string // Auto-generated sequence number for FIFO ordering
	DeduplicationHash      string // SHA-256 hash for content-based deduplication
}

type MessageAttribute struct {
	DataType    string
	StringValue string
	BinaryValue []byte
}

type VisibilityEntry struct {
	ReceiptHandle     string
	VisibilityTimeout int
}

type Queue struct {
	Name                          string
	URL                           string
	Attributes                    map[string]string
	VisibilityTimeoutSeconds      int
	MessageRetentionPeriod        int
	MaxReceiveCount               int
	DelaySeconds                  int
	ReceiveMessageWaitTimeSeconds int
	DeadLetterQueueName           string
	RedrivePolicy                 string
	CreatedAt                     time.Time

	// FIFO-specific fields
	FifoQueue                 bool   // True if this is a FIFO queue (.fifo suffix)
	ContentBasedDeduplication bool   // Enable content-based deduplication
	DeduplicationScope        string // "queue" or "messageGroup"
	FifoThroughputLimit       string // "perQueue" or "perMessageGroupId"
}

type Storage interface {
	// Queue operations
	CreateQueue(ctx context.Context, queue *Queue) error
	DeleteQueue(ctx context.Context, queueName string) error
	GetQueue(ctx context.Context, queueName string) (*Queue, error)
	ListQueues(ctx context.Context, prefix string) ([]*Queue, error)
	UpdateQueueAttributes(ctx context.Context, queueName string, attributes map[string]string) error

	// Message operations
	SendMessage(ctx context.Context, message *Message) error
	SendMessageBatch(ctx context.Context, messages []*Message) error
	ReceiveMessages(ctx context.Context, queueName string, maxMessages int, waitTimeSeconds int, visibilityTimeout int) ([]*Message, error)
	DeleteMessage(ctx context.Context, queueName string, receiptHandle string) error
	DeleteMessageBatch(ctx context.Context, queueName string, receiptHandles []string) error
	ChangeMessageVisibility(ctx context.Context, queueName string, receiptHandle string, visibilityTimeout int) error
	ChangeMessageVisibilityBatch(ctx context.Context, queueName string, entries []VisibilityEntry) error

	// FIFO deduplication
	CheckForDuplicate(ctx context.Context, queueName, deduplicationHash string, deduplicationWindow time.Duration) (bool, error)

	// DLQ operations
	MoveMessageToDLQ(ctx context.Context, message *Message, dlqName string) error
	GetExpiredMessages(ctx context.Context) ([]*Message, error)

	// Dashboard operations
	GetInFlightMessages(ctx context.Context, queueName string) ([]*Message, error)

	// Maintenance
	PurgeQueue(ctx context.Context, queueName string) error
	Close() error
}
