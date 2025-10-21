package api_test

import (
	"context"
	"strconv"
	"strings"
	"time"

	"sqs-bridge/src/storage"
)

// Mock storage for testing
type MockStorage struct {
	queues                map[string]*storage.Queue
	messages              map[string][]*storage.Message
	lastVisibilityTimeout int // Track the last visibility timeout used
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		queues:   make(map[string]*storage.Queue),
		messages: make(map[string][]*storage.Message),
	}
}

func (m *MockStorage) CreateQueue(ctx context.Context, queue *storage.Queue) error {
	m.queues[queue.Name] = queue
	return nil
}

func (m *MockStorage) DeleteQueue(ctx context.Context, queueName string) error {
	delete(m.queues, queueName)
	delete(m.messages, queueName)
	return nil
}

func (m *MockStorage) GetQueue(ctx context.Context, queueName string) (*storage.Queue, error) {
	if queue, exists := m.queues[queueName]; exists {
		return queue, nil
	}
	return nil, nil
}

func (m *MockStorage) ListQueues(ctx context.Context, prefix string) ([]*storage.Queue, error) {
	var result []*storage.Queue
	for _, queue := range m.queues {
		if prefix == "" || strings.HasPrefix(queue.Name, prefix) {
			result = append(result, queue)
		}
	}
	return result, nil
}

func (m *MockStorage) UpdateQueueAttributes(ctx context.Context, queueName string, attributes map[string]string) error {
	return nil
}

func (m *MockStorage) SendMessage(ctx context.Context, message *storage.Message) error {
	m.messages[message.QueueName] = append(m.messages[message.QueueName], message)
	return nil
}

func (m *MockStorage) SendMessageBatch(ctx context.Context, messages []*storage.Message) error {
	for _, msg := range messages {
		m.SendMessage(ctx, msg)
	}
	return nil
}

func (m *MockStorage) ReceiveMessages(ctx context.Context, queueName string, maxMessages int, waitTimeSeconds int, visibilityTimeout int) ([]*storage.Message, error) {
	// Store the visibility timeout for testing
	m.lastVisibilityTimeout = visibilityTimeout

	messages := m.messages[queueName]
	if len(messages) == 0 {
		return []*storage.Message{}, nil
	}

	now := time.Now()
	var availableMessages []*storage.Message

	// Find messages that are currently visible (not in flight)
	for _, msg := range messages {
		// Skip messages that are still in visibility timeout
		if msg.VisibleAt != nil && now.Before(*msg.VisibleAt) {
			continue
		}
		availableMessages = append(availableMessages, msg)
	}

	// Return up to maxMessages from available messages
	if len(availableMessages) > maxMessages {
		availableMessages = availableMessages[:maxMessages]
	}

	// Set visibility timeout on returned messages and update VisibleAt
	timeoutDuration := 30 // default
	if visibilityTimeout > 0 {
		timeoutDuration = visibilityTimeout
	}

	for _, msg := range availableMessages {
		msg.VisibilityTimeout = now.Add(time.Duration(timeoutDuration) * time.Second)
		msg.VisibleAt = &msg.VisibilityTimeout // Make message invisible until this time
		msg.ReceiveCount++
		msg.ReceiptHandle = "receipt-" + msg.ID + "-" + strconv.FormatInt(now.UnixNano(), 10) // New receipt handle
	}

	return availableMessages, nil
}

func (m *MockStorage) DeleteMessage(ctx context.Context, queueName string, receiptHandle string) error {
	messages := m.messages[queueName]
	for i, msg := range messages {
		if msg.ReceiptHandle == receiptHandle {
			m.messages[queueName] = append(messages[:i], messages[i+1:]...)
			break
		}
	}
	return nil
}

func (m *MockStorage) DeleteMessageBatch(ctx context.Context, queueName string, receiptHandles []string) error {
	for _, handle := range receiptHandles {
		m.DeleteMessage(ctx, queueName, handle)
	}
	return nil
}

func (m *MockStorage) ChangeMessageVisibility(ctx context.Context, queueName string, receiptHandle string, visibilityTimeout int) error {
	return nil
}

func (m *MockStorage) ChangeMessageVisibilityBatch(ctx context.Context, queueName string, entries []storage.VisibilityEntry) error {
	for _, entry := range entries {
		m.ChangeMessageVisibility(ctx, queueName, entry.ReceiptHandle, entry.VisibilityTimeout)
	}
	return nil
}

func (m *MockStorage) MoveMessageToDLQ(ctx context.Context, message *storage.Message, dlqName string) error {
	return nil
}

func (m *MockStorage) GetExpiredMessages(ctx context.Context) ([]*storage.Message, error) {
	return []*storage.Message{}, nil
}

func (m *MockStorage) GetInFlightMessages(ctx context.Context, queueName string) ([]*storage.Message, error) {
	return m.messages[queueName], nil
}

func (m *MockStorage) CheckForDuplicate(ctx context.Context, queueName, deduplicationHash string, deduplicationWindow time.Duration) (bool, error) {
	return false, nil // No duplicates in mock
}

func (m *MockStorage) PurgeQueue(ctx context.Context, queueName string) error {
	m.messages[queueName] = []*storage.Message{}
	return nil
}

func (m *MockStorage) Close() error {
	return nil
}

// Access key operations (mock implementations)
func (m *MockStorage) CreateAccessKey(ctx context.Context, accessKey *storage.AccessKey) error {
	return nil
}

func (m *MockStorage) GetAccessKey(ctx context.Context, accessKeyID string) (*storage.AccessKey, error) {
	return nil, nil
}

func (m *MockStorage) ListAccessKeys(ctx context.Context) ([]*storage.AccessKey, error) {
	return []*storage.AccessKey{}, nil
}

func (m *MockStorage) DeactivateAccessKey(ctx context.Context, accessKeyID string) error {
	return nil
}

func (m *MockStorage) DeleteAccessKey(ctx context.Context, accessKeyID string) error {
	return nil
}

func (m *MockStorage) UpdateAccessKeyUsage(ctx context.Context, accessKeyID string) error {
	return nil
}
