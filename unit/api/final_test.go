package api_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"sqs-bridge/src/api"
	"sqs-bridge/src/storage"
)

// Simple mock storage that doesn't implement visibility timeout behavior for testing
type SimpleMockStorage struct {
	queues                map[string]*storage.Queue
	messages              map[string][]*storage.Message
	lastVisibilityTimeout int
}

func NewSimpleMockStorage() *SimpleMockStorage {
	return &SimpleMockStorage{
		queues:   make(map[string]*storage.Queue),
		messages: make(map[string][]*storage.Message),
	}
}

func (m *SimpleMockStorage) CreateQueue(ctx context.Context, queue *storage.Queue) error {
	m.queues[queue.Name] = queue
	return nil
}

func (m *SimpleMockStorage) DeleteQueue(ctx context.Context, queueName string) error {
	delete(m.queues, queueName)
	delete(m.messages, queueName)
	return nil
}

func (m *SimpleMockStorage) GetQueue(ctx context.Context, queueName string) (*storage.Queue, error) {
	if queue, exists := m.queues[queueName]; exists {
		return queue, nil
	}
	return nil, nil
}

func (m *SimpleMockStorage) ListQueues(ctx context.Context, prefix string) ([]*storage.Queue, error) {
	var result []*storage.Queue
	for _, queue := range m.queues {
		if prefix == "" || strings.HasPrefix(queue.Name, prefix) {
			result = append(result, queue)
		}
	}
	return result, nil
}

func (m *SimpleMockStorage) UpdateQueueAttributes(ctx context.Context, queueName string, attributes map[string]string) error {
	return nil
}

func (m *SimpleMockStorage) SendMessage(ctx context.Context, message *storage.Message) error {
	m.messages[message.QueueName] = append(m.messages[message.QueueName], message)
	return nil
}

func (m *SimpleMockStorage) SendMessageBatch(ctx context.Context, messages []*storage.Message) error {
	for _, msg := range messages {
		m.SendMessage(ctx, msg)
	}
	return nil
}

// Simple implementation that always returns available messages (for testing)
func (m *SimpleMockStorage) ReceiveMessages(ctx context.Context, queueName string, maxMessages int, waitTimeSeconds int, visibilityTimeout int) ([]*storage.Message, error) {
	// Store the visibility timeout for testing
	m.lastVisibilityTimeout = visibilityTimeout

	messages := m.messages[queueName]
	if len(messages) == 0 {
		return []*storage.Message{}, nil
	}

	// Return up to maxMessages
	if len(messages) > maxMessages {
		messages = messages[:maxMessages]
	}

	// Set visibility timeout on messages but don't make them invisible
	now := time.Now()
	timeoutDuration := 30 // default
	if visibilityTimeout > 0 {
		timeoutDuration = visibilityTimeout
	}

	var result []*storage.Message
	for _, msg := range messages {
		// Create a copy to avoid modifying the original
		msgCopy := *msg
		msgCopy.VisibilityTimeout = now.Add(time.Duration(timeoutDuration) * time.Second)
		msgCopy.ReceiveCount++
		msgCopy.ReceiptHandle = "receipt-" + msg.ID + "-" + strconv.FormatInt(now.UnixNano(), 10)
		result = append(result, &msgCopy)
	}

	return result, nil
}

func (m *SimpleMockStorage) DeleteMessage(ctx context.Context, queueName string, receiptHandle string) error {
	messages := m.messages[queueName]
	for i, msg := range messages {
		if msg.ReceiptHandle == receiptHandle {
			m.messages[queueName] = append(messages[:i], messages[i+1:]...)
			break
		}
	}
	return nil
}

func (m *SimpleMockStorage) DeleteMessageBatch(ctx context.Context, queueName string, receiptHandles []string) error {
	for _, handle := range receiptHandles {
		m.DeleteMessage(ctx, queueName, handle)
	}
	return nil
}

func (m *SimpleMockStorage) ChangeMessageVisibility(ctx context.Context, queueName string, receiptHandle string, visibilityTimeout int) error {
	return nil
}

func (m *SimpleMockStorage) ChangeMessageVisibilityBatch(ctx context.Context, queueName string, entries []storage.VisibilityEntry) error {
	return nil
}

func (m *SimpleMockStorage) MoveMessageToDLQ(ctx context.Context, message *storage.Message, dlqName string) error {
	return nil
}

func (m *SimpleMockStorage) GetExpiredMessages(ctx context.Context) ([]*storage.Message, error) {
	return []*storage.Message{}, nil
}

func (m *SimpleMockStorage) GetInFlightMessages(ctx context.Context, queueName string) ([]*storage.Message, error) {
	return m.messages[queueName], nil
}

func (m *SimpleMockStorage) PurgeQueue(ctx context.Context, queueName string) error {
	m.messages[queueName] = []*storage.Message{}
	return nil
}

func (m *SimpleMockStorage) CheckForDuplicate(ctx context.Context, queueName, deduplicationHash string, deduplicationWindow time.Duration) (bool, error) {
	return false, nil // No duplicates in mock
}

func (m *SimpleMockStorage) Close() error {
	return nil
}

// Access key operations (mock implementations)
func (m *SimpleMockStorage) CreateAccessKey(ctx context.Context, accessKey *storage.AccessKey) error {
	return nil
}

func (m *SimpleMockStorage) GetAccessKey(ctx context.Context, accessKeyID string) (*storage.AccessKey, error) {
	return nil, nil
}

func (m *SimpleMockStorage) ListAccessKeys(ctx context.Context) ([]*storage.AccessKey, error) {
	return []*storage.AccessKey{}, nil
}

func (m *SimpleMockStorage) DeactivateAccessKey(ctx context.Context, accessKeyID string) error {
	return nil
}

func (m *SimpleMockStorage) DeleteAccessKey(ctx context.Context, accessKeyID string) error {
	return nil
}

func (m *SimpleMockStorage) UpdateAccessKeyUsage(ctx context.Context, accessKeyID string) error {
	return nil
}

func TestReceiveMessageVisibilityTimeoutFinal(t *testing.T) {
	tests := []struct {
		name              string
		visibilityTimeout string
		expectedTimeout   int
		description       string
	}{
		{
			name:              "default_visibility_timeout",
			visibilityTimeout: "",
			expectedTimeout:   0,
			description:       "When no VisibilityTimeout is provided, should use queue default",
		},
		{
			name:              "custom_visibility_timeout_60",
			visibilityTimeout: "60",
			expectedTimeout:   60,
			description:       "Should use provided VisibilityTimeout of 60 seconds",
		},
		{
			name:              "custom_visibility_timeout_300",
			visibilityTimeout: "300",
			expectedTimeout:   300,
			description:       "Should use provided VisibilityTimeout of 300 seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh storage for each test
			mockStorage := NewSimpleMockStorage()
			handler := api.NewSQSHandler(mockStorage, "http://localhost:9324", "test_admin", "test_password")

			// Create test queue
			queue := &storage.Queue{
				Name:                     "test-queue",
				URL:                      "http://localhost:9324/test-queue",
				VisibilityTimeoutSeconds: 30,
				CreatedAt:                time.Now(),
			}
			mockStorage.CreateQueue(context.Background(), queue)

			// Add test message
			message := &storage.Message{
				ID:            "test-msg-" + tt.name,
				QueueName:     "test-queue",
				Body:          "test message",
				ReceiptHandle: "test-receipt-" + tt.name,
				CreatedAt:     time.Now(),
			}
			mockStorage.SendMessage(context.Background(), message)

			// Create request
			formData := url.Values{}
			formData.Set("Action", "ReceiveMessage")
			formData.Set("QueueUrl", "http://localhost:9324/test-queue")
			if tt.visibilityTimeout != "" {
				formData.Set("VisibilityTimeout", tt.visibilityTimeout)
			}

			req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Execute request
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// Verify request was successful
			if w.Code != 200 {
				t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
				return
			}

			// Verify that the correct visibility timeout was passed to storage
			if mockStorage.lastVisibilityTimeout != tt.expectedTimeout {
				t.Errorf("%s: Expected visibility timeout %d, got %d",
					tt.description, tt.expectedTimeout, mockStorage.lastVisibilityTimeout)
			}

			// Verify XML response structure
			responseBody := w.Body.String()
			if !strings.Contains(responseBody, "ReceiveMessageResponse") {
				t.Errorf("Response should contain ReceiveMessageResponse element. Got: %s", responseBody)
			}

			// Verify message is present in response
			if !strings.Contains(responseBody, "test message") {
				t.Errorf("Response should contain message body. Got: %s", responseBody)
			}
		})
	}
}
