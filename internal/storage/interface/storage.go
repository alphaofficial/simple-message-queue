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
}

type MessageAttribute struct {
	DataType    string
	StringValue string
	BinaryValue []byte
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
	ReceiveMessages(ctx context.Context, queueName string, maxMessages int, waitTimeSeconds int) ([]*Message, error)
	DeleteMessage(ctx context.Context, queueName string, receiptHandle string) error
	DeleteMessageBatch(ctx context.Context, queueName string, receiptHandles []string) error
	ChangeMessageVisibility(ctx context.Context, queueName string, receiptHandle string, visibilityTimeout int) error

	// DLQ operations
	MoveMessageToDLQ(ctx context.Context, message *Message, dlqName string) error
	GetExpiredMessages(ctx context.Context) ([]*Message, error)

	// Dashboard operations
	GetInFlightMessages(ctx context.Context, queueName string) ([]*Message, error)

	// Maintenance
	PurgeQueue(ctx context.Context, queueName string) error
	Close() error
}
