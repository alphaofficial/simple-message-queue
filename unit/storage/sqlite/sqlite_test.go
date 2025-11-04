package sqlite_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"simple-message-queue/src/storage"
	"simple-message-queue/src/storage/sqlite"
)

func setupTestDB(t *testing.T) *sqlite.SQLiteStorage {
	tempDir, err := os.MkdirTemp("", "sqs_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	store, err := sqlite.NewSQLiteStorage(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create SQLite storage: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
		os.RemoveAll(tempDir)
	})

	return store
}

func TestReceiveMessagesVisibilityTimeout(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	queue := &storage.Queue{
		Name:                     "test-queue",
		URL:                      "http://localhost:9324/test-queue",
		VisibilityTimeoutSeconds: 30,
		CreatedAt:                time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	messages := []*storage.Message{
		{
			ID:        "msg-1",
			QueueName: "test-queue",
			Body:      "Test message 1",
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg-2",
			QueueName: "test-queue",
			Body:      "Test message 2",
			CreatedAt: time.Now(),
		},
	}

	for _, msg := range messages {
		err := store.SendMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
	}

	tests := []struct {
		name              string
		visibilityTimeout int
		expectedDuration  time.Duration
		description       string
	}{
		{
			name:              "use_queue_default",
			visibilityTimeout: 0,
			expectedDuration:  30 * time.Second,
			description:       "When visibilityTimeout is 0, should use queue default (30 seconds)",
		},
		{
			name:              "use_custom_timeout_60",
			visibilityTimeout: 60,
			expectedDuration:  60 * time.Second,
			description:       "Should use custom visibility timeout of 60 seconds",
		},
		{
			name:              "use_custom_timeout_120",
			visibilityTimeout: 120,
			expectedDuration:  120 * time.Second,
			description:       "Should use custom visibility timeout of 120 seconds",
		},
		{
			name:              "use_custom_timeout_600",
			visibilityTimeout: 600,
			expectedDuration:  600 * time.Second,
			description:       "Should use custom visibility timeout of 600 seconds (10 minutes)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testMessages := []*storage.Message{
				{
					ID:        fmt.Sprintf("msg-1-%s", tt.name),
					QueueName: "test-queue",
					Body:      "Test message 1",
					CreatedAt: time.Now(),
				},
				{
					ID:        fmt.Sprintf("msg-2-%s", tt.name),
					QueueName: "test-queue",
					Body:      "Test message 2",
					CreatedAt: time.Now(),
				},
			}

			for _, msg := range testMessages {
				err := store.SendMessage(ctx, msg)
				if err != nil {
					t.Fatalf("Failed to send message: %v", err)
				}
			}

			startTime := time.Now()

			receivedMessages, err := store.ReceiveMessages(ctx, "test-queue", 10, 0, tt.visibilityTimeout)
			if err != nil {
				t.Fatalf("Failed to receive messages: %v", err)
			}

			if len(receivedMessages) == 0 {
				t.Fatalf("Expected to receive messages, got none")
			}

			for i, msg := range receivedMessages {
				actualDuration := msg.VisibilityTimeout.Sub(startTime)

				timeDiff := actualDuration - tt.expectedDuration
				if timeDiff < 0 {
					timeDiff = -timeDiff
				}

				if timeDiff > time.Second {
					t.Errorf("%s: Message %d visibility timeout mismatch. Expected duration ~%v, got %v (diff: %v)",
						tt.description, i, tt.expectedDuration, actualDuration, timeDiff)
				}

				if msg.ReceiveCount == 0 {
					t.Errorf("Message receive count should be incremented")
				}

				if msg.ReceiptHandle == "" {
					t.Errorf("Message should have receipt handle")
				}
			}

			invisibleMessages, err := store.ReceiveMessages(ctx, "test-queue", 10, 0, tt.visibilityTimeout)
			if err != nil {
				t.Fatalf("Failed to receive messages on second attempt: %v", err)
			}

			if len(invisibleMessages) != 0 {
				t.Errorf("Expected no visible messages immediately after receive, got %d", len(invisibleMessages))
			}
		})
	}
}

func TestReceiveMessagesVisibilityTimeoutInDatabase(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	queue := &storage.Queue{
		Name:                     "test-queue",
		URL:                      "http://localhost:9324/test-queue",
		VisibilityTimeoutSeconds: 30,
		CreatedAt:                time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	message := &storage.Message{
		ID:        "msg-1",
		QueueName: "test-queue",
		Body:      "Test message",
		CreatedAt: time.Now(),
	}
	err = store.SendMessage(ctx, message)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	visibilityTimeout := 300 // 5 minutes
	startTime := time.Now()

	receivedMessages, err := store.ReceiveMessages(ctx, "test-queue", 1, 0, visibilityTimeout)
	if err != nil {
		t.Fatalf("Failed to receive messages: %v", err)
	}

	if len(receivedMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(receivedMessages))
	}

	receivedMessage := receivedMessages[0]

	if receivedMessage.ID != message.ID {
		t.Errorf("Expected message ID %s, got %s", message.ID, receivedMessage.ID)
	}

	if receivedMessage.ReceiptHandle == "" {
		t.Error("Receipt handle should not be empty")
	}

	expectedTimeout := startTime.Add(time.Duration(visibilityTimeout) * time.Second)
	timeDiff := receivedMessage.VisibilityTimeout.Sub(expectedTimeout)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	if timeDiff > time.Second {
		t.Errorf("Message visibility timeout mismatch. Expected ~%v, got %v (diff: %v)",
			expectedTimeout, receivedMessage.VisibilityTimeout, timeDiff)
	}
}

func TestReceiveMessagesQueueDefault(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	queue := &storage.Queue{
		Name:                     "test-queue",
		URL:                      "http://localhost:9324/test-queue",
		VisibilityTimeoutSeconds: 45, // Custom default
		CreatedAt:                time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	message := &storage.Message{
		ID:        "msg-1",
		QueueName: "test-queue",
		Body:      "Test message",
		CreatedAt: time.Now(),
	}
	err = store.SendMessage(ctx, message)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	startTime := time.Now()
	receivedMessages, err := store.ReceiveMessages(ctx, "test-queue", 1, 0, 0) // 0 = use queue default
	if err != nil {
		t.Fatalf("Failed to receive messages: %v", err)
	}

	if len(receivedMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(receivedMessages))
	}

	expectedDuration := 30 * time.Second
	actualDuration := receivedMessages[0].VisibilityTimeout.Sub(startTime)

	timeDiff := actualDuration - expectedDuration
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	if timeDiff > time.Second {
		t.Errorf("Expected to use 30 second default timeout, got %v (diff: %v)", actualDuration, timeDiff)
	}
}

func TestReceiveMessagesEmptyQueue(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	queue := &storage.Queue{
		Name:                     "empty-queue",
		URL:                      "http://localhost:9324/empty-queue",
		VisibilityTimeoutSeconds: 30,
		CreatedAt:                time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	receivedMessages, err := store.ReceiveMessages(ctx, "empty-queue", 10, 0, 60)
	if err != nil {
		t.Fatalf("Failed to receive messages from empty queue: %v", err)
	}

	if len(receivedMessages) != 0 {
		t.Errorf("Expected no messages from empty queue, got %d", len(receivedMessages))
	}
}

func TestReceiveMessagesMaxMessages(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	queue := &storage.Queue{
		Name:                     "test-queue",
		URL:                      "http://localhost:9324/test-queue",
		VisibilityTimeoutSeconds: 30,
		CreatedAt:                time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	for i := 0; i < 10; i++ {
		message := &storage.Message{
			ID:        "msg-" + string(rune(i+'0')),
			QueueName: "test-queue",
			Body:      "Test message " + string(rune(i+'0')),
			CreatedAt: time.Now(),
		}
		err := store.SendMessage(ctx, message)
		if err != nil {
			t.Fatalf("Failed to send message %d: %v", i, err)
		}
	}

	tests := []struct {
		name              string
		maxMessages       int
		expectedCount     int
		visibilityTimeout int
	}{
		{
			name:              "receive_1_message",
			maxMessages:       1,
			expectedCount:     1,
			visibilityTimeout: 60,
		},
		{
			name:              "receive_5_messages",
			maxMessages:       5,
			expectedCount:     5,
			visibilityTimeout: 120,
		},
		{
			name:              "receive_all_remaining",
			maxMessages:       10,
			expectedCount:     4, // Only 4 left after previous tests
			visibilityTimeout: 180,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime := time.Now()
			receivedMessages, err := store.ReceiveMessages(ctx, "test-queue", tt.maxMessages, 0, tt.visibilityTimeout)
			if err != nil {
				t.Fatalf("Failed to receive messages: %v", err)
			}

			if len(receivedMessages) != tt.expectedCount {
				t.Errorf("Expected %d messages, got %d", tt.expectedCount, len(receivedMessages))
			}

			expectedDuration := time.Duration(tt.visibilityTimeout) * time.Second
			for i, msg := range receivedMessages {
				actualDuration := msg.VisibilityTimeout.Sub(startTime)
				timeDiff := actualDuration - expectedDuration
				if timeDiff < 0 {
					timeDiff = -timeDiff
				}

				if timeDiff > time.Second {
					t.Errorf("Message %d visibility timeout mismatch. Expected ~%v, got %v",
						i, expectedDuration, actualDuration)
				}
			}
		})
	}
}
