package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"sqs-backend/src/storage"
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

func TestReceiveMessageVisibilityTimeout(t *testing.T) {
	// Setup
	mockStorage := NewMockStorage()
	handler := NewSQSHandler(mockStorage, "http://localhost:9324")

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
		ID:            "test-msg-1",
		QueueName:     "test-queue",
		Body:          "test message",
		ReceiptHandle: "test-receipt-1",
		CreatedAt:     time.Now(),
	}
	mockStorage.SendMessage(context.Background(), message)

	tests := []struct {
		name              string
		visibilityTimeout string
		expectedTimeout   int
		description       string
	}{
		{
			name:              "default_visibility_timeout",
			visibilityTimeout: "",
			expectedTimeout:   0, // Should use queue default
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
		{
			name:              "zero_visibility_timeout",
			visibilityTimeout: "0",
			expectedTimeout:   0,
			description:       "Should use queue default when VisibilityTimeout is 0",
		},
		{
			name:              "max_visibility_timeout",
			visibilityTimeout: "43200", // 12 hours
			expectedTimeout:   43200,
			description:       "Should accept maximum VisibilityTimeout of 12 hours",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock state
			mockStorage.lastVisibilityTimeout = -1

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
			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
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
		})
	}
}

func TestReceiveMessageVisibilityTimeoutValidation(t *testing.T) {
	// Setup
	mockStorage := NewMockStorage()
	handler := NewSQSHandler(mockStorage, "http://localhost:9324")

	// Create test queue
	queue := &storage.Queue{
		Name:                     "test-queue",
		URL:                      "http://localhost:9324/test-queue",
		VisibilityTimeoutSeconds: 30,
		CreatedAt:                time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	tests := []struct {
		name              string
		visibilityTimeout string
		expectedTimeout   int
		description       string
	}{
		{
			name:              "invalid_negative_timeout",
			visibilityTimeout: "-10",
			expectedTimeout:   0, // Should fallback to queue default
			description:       "Negative timeout should be ignored and use queue default",
		},
		{
			name:              "invalid_too_large_timeout",
			visibilityTimeout: "50000", // > 12 hours
			expectedTimeout:   0,       // Should fallback to queue default
			description:       "Timeout larger than 12 hours should be ignored",
		},
		{
			name:              "invalid_non_numeric_timeout",
			visibilityTimeout: "invalid",
			expectedTimeout:   0, // Should fallback to queue default
			description:       "Non-numeric timeout should be ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock state
			mockStorage.lastVisibilityTimeout = -1

			// Create request
			formData := url.Values{}
			formData.Set("Action", "ReceiveMessage")
			formData.Set("QueueUrl", "http://localhost:9324/test-queue")
			formData.Set("VisibilityTimeout", tt.visibilityTimeout)

			req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Execute request
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// Verify request was successful (invalid values should not cause errors)
			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
				return
			}

			// Verify that invalid timeouts fallback to queue default (0)
			if mockStorage.lastVisibilityTimeout != tt.expectedTimeout {
				t.Errorf("%s: Expected visibility timeout %d, got %d",
					tt.description, tt.expectedTimeout, mockStorage.lastVisibilityTimeout)
			}
		})
	}
}

func TestReceiveMessageParameterHandling(t *testing.T) {
	// Setup
	mockStorage := NewMockStorage()
	handler := NewSQSHandler(mockStorage, "http://localhost:9324")

	// Create test queue
	queue := &storage.Queue{
		Name:                     "test-queue",
		URL:                      "http://localhost:9324/test-queue",
		VisibilityTimeoutSeconds: 30,
		CreatedAt:                time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	// Add test messages
	for i := 0; i < 5; i++ {
		message := &storage.Message{
			ID:            "test-msg-" + strconv.Itoa(i),
			QueueName:     "test-queue",
			Body:          "test message " + strconv.Itoa(i),
			ReceiptHandle: "test-receipt-" + strconv.Itoa(i),
			CreatedAt:     time.Now(),
		}
		mockStorage.SendMessage(context.Background(), message)
	}

	t.Run("all_parameters_together", func(t *testing.T) {
		// Reset mock state
		mockStorage.lastVisibilityTimeout = -1

		// Create request with all parameters
		formData := url.Values{}
		formData.Set("Action", "ReceiveMessage")
		formData.Set("QueueUrl", "http://localhost:9324/test-queue")
		formData.Set("MaxNumberOfMessages", "3")
		formData.Set("WaitTimeSeconds", "5")
		formData.Set("VisibilityTimeout", "120")

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// Execute request
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		// Verify request was successful
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		// Verify that the correct visibility timeout was used
		if mockStorage.lastVisibilityTimeout != 120 {
			t.Errorf("Expected visibility timeout 120, got %d", mockStorage.lastVisibilityTimeout)
		}

		// Verify XML response contains messages
		responseBody := w.Body.String()
		if !strings.Contains(responseBody, "ReceiveMessageResponse") {
			t.Errorf("Response should contain ReceiveMessageResponse element")
		}
	})
}
