package sqlite_test

import (
	"context"
	"testing"
	"time"

	"simple-message-queue/src/storage"
)

func TestDeleteQueueCascadesDLQ(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create a Dead Letter Queue first
	dlq := &storage.Queue{
		Name:      "test-dlq",
		URL:       "http://localhost:9324/test-dlq",
		CreatedAt: time.Now(),
	}
	err := store.CreateQueue(ctx, dlq)
	if err != nil {
		t.Fatalf("Failed to create DLQ: %v", err)
	}

	// Create a main queue that uses the DLQ
	mainQueue := &storage.Queue{
		Name:                "test-main-queue",
		URL:                 "http://localhost:9324/test-main-queue",
		MaxReceiveCount:     3,
		DeadLetterQueueName: "test-dlq",
		CreatedAt:           time.Now(),
	}
	err = store.CreateQueue(ctx, mainQueue)
	if err != nil {
		t.Fatalf("Failed to create main queue: %v", err)
	}

	// Add some messages to both queues
	mainMessage := &storage.Message{
		ID:        "main-msg-1",
		QueueName: "test-main-queue",
		Body:      "Test message in main queue",
		CreatedAt: time.Now(),
	}
	err = store.SendMessage(ctx, mainMessage)
	if err != nil {
		t.Fatalf("Failed to send message to main queue: %v", err)
	}

	dlqMessage := &storage.Message{
		ID:        "dlq-msg-1",
		QueueName: "test-dlq",
		Body:      "Test message in DLQ",
		CreatedAt: time.Now(),
	}
	err = store.SendMessage(ctx, dlqMessage)
	if err != nil {
		t.Fatalf("Failed to send message to DLQ: %v", err)
	}

	// Verify both queues exist
	mainQueueCheck, err := store.GetQueue(ctx, "test-main-queue")
	if err != nil {
		t.Fatalf("Failed to get main queue: %v", err)
	}
	if mainQueueCheck == nil {
		t.Fatal("Main queue should exist")
	}

	dlqCheck, err := store.GetQueue(ctx, "test-dlq")
	if err != nil {
		t.Fatalf("Failed to get DLQ: %v", err)
	}
	if dlqCheck == nil {
		t.Fatal("DLQ should exist")
	}

	// Delete the main queue - this should also delete the DLQ
	err = store.DeleteQueue(ctx, "test-main-queue")
	if err != nil {
		t.Fatalf("Failed to delete main queue: %v", err)
	}

	// Verify main queue is deleted
	mainQueueCheck, err = store.GetQueue(ctx, "test-main-queue")
	if err != nil {
		t.Fatalf("Error checking for deleted main queue: %v", err)
	}
	if mainQueueCheck != nil {
		t.Error("Main queue should be deleted")
	}

	// Verify DLQ is also deleted (cascade deletion)
	dlqCheck, err = store.GetQueue(ctx, "test-dlq")
	if err != nil {
		t.Fatalf("Error checking for deleted DLQ: %v", err)
	}
	if dlqCheck != nil {
		t.Error("DLQ should be deleted when main queue is deleted")
	}
}

func TestDeleteQueueWithoutDLQ(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create a queue without a DLQ
	queue := &storage.Queue{
		Name:      "test-queue-no-dlq",
		URL:       "http://localhost:9324/test-queue-no-dlq",
		CreatedAt: time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Add a message to the queue
	message := &storage.Message{
		ID:        "msg-1",
		QueueName: "test-queue-no-dlq",
		Body:      "Test message",
		CreatedAt: time.Now(),
	}
	err = store.SendMessage(ctx, message)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify queue exists
	queueCheck, err := store.GetQueue(ctx, "test-queue-no-dlq")
	if err != nil {
		t.Fatalf("Failed to get queue: %v", err)
	}
	if queueCheck == nil {
		t.Fatal("Queue should exist")
	}

	// Delete the queue
	err = store.DeleteQueue(ctx, "test-queue-no-dlq")
	if err != nil {
		t.Fatalf("Failed to delete queue: %v", err)
	}

	// Verify queue is deleted
	queueCheck, err = store.GetQueue(ctx, "test-queue-no-dlq")
	if err != nil {
		t.Fatalf("Error checking for deleted queue: %v", err)
	}
	if queueCheck != nil {
		t.Error("Queue should be deleted")
	}
}

func TestDeleteQueueWithNonExistentDLQ(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create a queue that references a non-existent DLQ
	queue := &storage.Queue{
		Name:                "test-queue-broken-dlq",
		URL:                 "http://localhost:9324/test-queue-broken-dlq",
		MaxReceiveCount:     3,
		DeadLetterQueueName: "non-existent-dlq", // This DLQ doesn't exist
		CreatedAt:           time.Now(),
	}
	err := store.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Verify queue exists
	queueCheck, err := store.GetQueue(ctx, "test-queue-broken-dlq")
	if err != nil {
		t.Fatalf("Failed to get queue: %v", err)
	}
	if queueCheck == nil {
		t.Fatal("Queue should exist")
	}

	// Delete the queue - this should not fail even if DLQ doesn't exist
	err = store.DeleteQueue(ctx, "test-queue-broken-dlq")
	if err != nil {
		t.Fatalf("Failed to delete queue with non-existent DLQ: %v", err)
	}

	// Verify queue is deleted
	queueCheck, err = store.GetQueue(ctx, "test-queue-broken-dlq")
	if err != nil {
		t.Fatalf("Error checking for deleted queue: %v", err)
	}
	if queueCheck != nil {
		t.Error("Queue should be deleted")
	}
}
