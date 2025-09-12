package sqlite_test

import (
	"context"
	"testing"
	"time"

	"sqs-backend/src/storage"
)

func TestSendMessageBatch(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create test queue
	queue := &storage.Queue{
		Name:      "test-batch-queue",
		URL:       "http://localhost:9324/test-batch-queue",
		CreatedAt: time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create batch of messages
	messages := []*storage.Message{
		{
			ID:        "batch-msg-1",
			QueueName: "test-batch-queue",
			Body:      "Batch message 1",
			CreatedAt: time.Now(),
		},
		{
			ID:        "batch-msg-2",
			QueueName: "test-batch-queue",
			Body:      "Batch message 2",
			CreatedAt: time.Now(),
		},
		{
			ID:        "batch-msg-3",
			QueueName: "test-batch-queue",
			Body:      "Batch message 3",
			CreatedAt: time.Now(),
		},
	}

	// Send messages in batch
	err = store.SendMessageBatch(ctx, messages)
	if err != nil {
		t.Fatalf("Failed to send message batch: %v", err)
	}

	// Verify messages were stored
	allMessages, err := store.GetInFlightMessages(ctx, "test-batch-queue")
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	if len(allMessages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(allMessages))
	}

	// Verify message content
	expectedBodies := map[string]string{
		"batch-msg-1": "Batch message 1",
		"batch-msg-2": "Batch message 2",
		"batch-msg-3": "Batch message 3",
	}

	for _, msg := range allMessages {
		expectedBody, exists := expectedBodies[msg.ID]
		if !exists {
			t.Errorf("Unexpected message ID: %s", msg.ID)
			continue
		}

		if msg.Body != expectedBody {
			t.Errorf("Message %s body mismatch. Expected: %s, Got: %s", msg.ID, expectedBody, msg.Body)
		}

		if msg.QueueName != "test-batch-queue" {
			t.Errorf("Message %s queue mismatch. Expected: test-batch-queue, Got: %s", msg.ID, msg.QueueName)
		}
	}
}

func TestDeleteMessageBatch(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create test queue
	queue := &storage.Queue{
		Name:      "test-delete-batch-queue",
		URL:       "http://localhost:9324/test-delete-batch-queue",
		CreatedAt: time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create and send test messages
	messages := []*storage.Message{
		{
			ID:            "del-msg-1",
			QueueName:     "test-delete-batch-queue",
			Body:          "Delete message 1",
			ReceiptHandle: "receipt-handle-1",
			CreatedAt:     time.Now(),
		},
		{
			ID:            "del-msg-2",
			QueueName:     "test-delete-batch-queue",
			Body:          "Delete message 2",
			ReceiptHandle: "receipt-handle-2",
			CreatedAt:     time.Now(),
		},
		{
			ID:            "del-msg-3",
			QueueName:     "test-delete-batch-queue",
			Body:          "Delete message 3",
			ReceiptHandle: "receipt-handle-3",
			CreatedAt:     time.Now(),
		},
	}

	for _, msg := range messages {
		err := store.SendMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Failed to send message %s: %v", msg.ID, err)
		}
	}

	// Verify initial count
	allMessages, err := store.GetInFlightMessages(ctx, "test-delete-batch-queue")
	if err != nil {
		t.Fatalf("Failed to get initial messages: %v", err)
	}
	if len(allMessages) != 3 {
		t.Fatalf("Expected 3 initial messages, got %d", len(allMessages))
	}

	// Delete messages 1 and 3 in batch
	receiptHandles := []string{"receipt-handle-1", "receipt-handle-3"}
	err = store.DeleteMessageBatch(ctx, "test-delete-batch-queue", receiptHandles)
	if err != nil {
		t.Fatalf("Failed to delete message batch: %v", err)
	}

	// Verify remaining messages
	remainingMessages, err := store.GetInFlightMessages(ctx, "test-delete-batch-queue")
	if err != nil {
		t.Fatalf("Failed to get remaining messages: %v", err)
	}

	if len(remainingMessages) != 1 {
		t.Errorf("Expected 1 remaining message, got %d", len(remainingMessages))
	}

	if len(remainingMessages) > 0 {
		remaining := remainingMessages[0]
		if remaining.ID != "del-msg-2" {
			t.Errorf("Wrong message remained. Expected del-msg-2, got %s", remaining.ID)
		}
		if remaining.ReceiptHandle != "receipt-handle-2" {
			t.Errorf("Wrong receipt handle remained. Expected receipt-handle-2, got %s", remaining.ReceiptHandle)
		}
	}
}

func TestChangeMessageVisibilityBatch(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create test queue
	queue := &storage.Queue{
		Name:      "test-visibility-batch-queue",
		URL:       "http://localhost:9324/test-visibility-batch-queue",
		CreatedAt: time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Create test messages
	messages := []*storage.Message{
		{
			ID:            "vis-msg-1",
			QueueName:     "test-visibility-batch-queue",
			Body:          "Visibility message 1",
			ReceiptHandle: "vis-receipt-1",
			CreatedAt:     time.Now(),
		},
		{
			ID:            "vis-msg-2",
			QueueName:     "test-visibility-batch-queue",
			Body:          "Visibility message 2",
			ReceiptHandle: "vis-receipt-2",
			CreatedAt:     time.Now(),
		},
	}

	for _, msg := range messages {
		err := store.SendMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Failed to send message %s: %v", msg.ID, err)
		}
	}

	// Create visibility entries for batch update
	visibilityEntries := []storage.VisibilityEntry{
		{
			ReceiptHandle:     "vis-receipt-1",
			VisibilityTimeout: 300, // 5 minutes
		},
		{
			ReceiptHandle:     "vis-receipt-2",
			VisibilityTimeout: 600, // 10 minutes
		},
	}

	// Update visibility in batch
	err = store.ChangeMessageVisibilityBatch(ctx, "test-visibility-batch-queue", visibilityEntries)
	if err != nil {
		t.Fatalf("Failed to change message visibility batch: %v", err)
	}

	// Verify the operation completed successfully by checking there are no errors
	// The actual visibility timeout values are verified in the individual message visibility tests
}

func TestBatchOperationTransactions(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create test queue
	queue := &storage.Queue{
		Name:      "test-transaction-queue",
		URL:       "http://localhost:9324/test-transaction-queue",
		CreatedAt: time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	t.Run("empty_batch_operations", func(t *testing.T) {
		// Test empty batch operations don't fail
		err := store.SendMessageBatch(ctx, []*storage.Message{})
		if err != nil {
			t.Errorf("Empty SendMessageBatch should not fail: %v", err)
		}

		err = store.DeleteMessageBatch(ctx, "test-transaction-queue", []string{})
		if err != nil {
			t.Errorf("Empty DeleteMessageBatch should not fail: %v", err)
		}

		err = store.ChangeMessageVisibilityBatch(ctx, "test-transaction-queue", []storage.VisibilityEntry{})
		if err != nil {
			t.Errorf("Empty ChangeMessageVisibilityBatch should not fail: %v", err)
		}
	})

	t.Run("large_batch_operations", func(t *testing.T) {
		// Test with maximum batch size (10 items)
		var messages []*storage.Message
		var receiptHandles []string
		var visibilityEntries []storage.VisibilityEntry

		for i := 0; i < 10; i++ {
			msg := &storage.Message{
				ID:            "large-batch-msg-" + string(rune('0'+i)),
				QueueName:     "test-transaction-queue",
				Body:          "Large batch message " + string(rune('0'+i)),
				ReceiptHandle: "large-receipt-" + string(rune('0'+i)),
				CreatedAt:     time.Now(),
			}
			messages = append(messages, msg)
			receiptHandles = append(receiptHandles, msg.ReceiptHandle)
			visibilityEntries = append(visibilityEntries, storage.VisibilityEntry{
				ReceiptHandle:     msg.ReceiptHandle,
				VisibilityTimeout: 300 + (i * 60), // Different timeouts
			})
		}

		// Send large batch
		err := store.SendMessageBatch(ctx, messages)
		if err != nil {
			t.Fatalf("Failed to send large message batch: %v", err)
		}

		// Verify all messages were sent
		allMessages, err := store.GetInFlightMessages(ctx, "test-transaction-queue")
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}

		if len(allMessages) != 10 {
			t.Errorf("Expected 10 messages, got %d", len(allMessages))
		}

		// Test large visibility batch
		err = store.ChangeMessageVisibilityBatch(ctx, "test-transaction-queue", visibilityEntries)
		if err != nil {
			t.Fatalf("Failed to change visibility for large batch: %v", err)
		}

		// Test large delete batch
		err = store.DeleteMessageBatch(ctx, "test-transaction-queue", receiptHandles)
		if err != nil {
			t.Fatalf("Failed to delete large message batch: %v", err)
		}

		// Verify all messages were deleted
		remainingMessages, err := store.GetInFlightMessages(ctx, "test-transaction-queue")
		if err != nil {
			t.Fatalf("Failed to get remaining messages: %v", err)
		}

		if len(remainingMessages) != 0 {
			t.Errorf("Expected 0 remaining messages, got %d", len(remainingMessages))
		}
	})
}
