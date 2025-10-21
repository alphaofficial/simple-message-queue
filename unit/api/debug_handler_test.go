package api_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sqs-bridge/src/api"
	"sqs-bridge/src/storage"
)

func TestHandlerDebugging(t *testing.T) {
	// Create fresh mock storage
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost:9324", "test_admin", "test_password")

	// Create test queue
	queue := &storage.Queue{
		Name:      "debug-handler-queue",
		URL:       "http://localhost:9324/debug-handler-queue",
		CreatedAt: time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	// Add test message
	message := &storage.Message{
		ID:        "debug-handler-msg",
		QueueName: "debug-handler-queue",
		Body:      "debug handler message",
		CreatedAt: time.Now(),
	}
	mockStorage.SendMessage(context.Background(), message)

	// Verify message exists in storage with correct queue name
	messages := mockStorage.messages["debug-handler-queue"]
	t.Logf("Messages in storage for queue 'debug-handler-queue': %d", len(messages))
	if len(messages) > 0 {
		t.Logf("Message: ID=%s, QueueName=%s, Body=%s",
			messages[0].ID, messages[0].QueueName, messages[0].Body)
	}

	// Test direct storage call
	receivedMsgs, err := mockStorage.ReceiveMessages(context.Background(), "debug-handler-queue", 1, 0, 0)
	if err != nil {
		t.Fatalf("Direct storage call failed: %v", err)
	}
	t.Logf("Direct storage call returned %d messages", len(receivedMsgs))

	// Test handler with minimal request
	formData := url.Values{}
	formData.Set("Action", "ReceiveMessage")
	formData.Set("QueueUrl", "http://localhost:9324/debug-handler-queue")

	req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	t.Logf("Handler response status: %d", w.Code)
	t.Logf("Handler response body: %s", w.Body.String())
	t.Logf("Handler response headers: %v", w.Header())

	// Test if the queue lookup is working
	fetchedQueue, err := mockStorage.GetQueue(context.Background(), "debug-handler-queue")
	if err != nil {
		t.Errorf("Failed to get queue: %v", err)
	} else if fetchedQueue == nil {
		t.Errorf("Queue not found in storage")
	} else {
		t.Logf("Queue found: Name=%s, URL=%s", fetchedQueue.Name, fetchedQueue.URL)
	}
}
