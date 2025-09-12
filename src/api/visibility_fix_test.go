package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sqs-backend/src/storage"
)

func TestVisibilityTimeoutSimple(t *testing.T) {
	// Setup fresh mock storage
	mockStorage := NewMockStorage()
	handler := NewSQSHandler(mockStorage, "http://localhost:9324")

	// Create test queue
	queue := &storage.Queue{
		Name:                     "simple-test-queue",
		URL:                      "http://localhost:9324/simple-test-queue",
		VisibilityTimeoutSeconds: 30,
		CreatedAt:                time.Now(),
	}
	err := mockStorage.CreateQueue(context.Background(), queue)
	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}

	// Add a test message
	message := &storage.Message{
		ID:            "simple-msg-1",
		QueueName:     "simple-test-queue",
		Body:          "simple test message",
		ReceiptHandle: "simple-receipt-1",
		CreatedAt:     time.Now(),
	}
	err = mockStorage.SendMessage(context.Background(), message)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify message was stored
	messages := mockStorage.messages["simple-test-queue"]
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message in storage, got %d", len(messages))
	}

	t.Logf("Message in storage: ID=%s, Body=%s", messages[0].ID, messages[0].Body)

	// Test ReceiveMessage with custom visibility timeout
	formData := url.Values{}
	formData.Set("Action", "ReceiveMessage")
	formData.Set("QueueUrl", "http://localhost:9324/simple-test-queue")
	formData.Set("VisibilityTimeout", "120")

	req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	t.Logf("Response Status: %d", w.Code)
	t.Logf("Response Body: %s", w.Body.String())
	t.Logf("Mock storage lastVisibilityTimeout: %d", mockStorage.lastVisibilityTimeout)

	// Verify basic functionality
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check if visibility timeout was passed to storage
	if mockStorage.lastVisibilityTimeout != 120 {
		t.Errorf("Expected visibility timeout 120, got %d", mockStorage.lastVisibilityTimeout)
	}

	// Check response contains message data
	responseBody := w.Body.String()
	if strings.Contains(responseBody, "ReceiveMessageResponse") {
		t.Logf("SUCCESS: Found ReceiveMessageResponse in response")

		// Check if message body is present
		if strings.Contains(responseBody, "simple test message") {
			t.Logf("SUCCESS: Found message body in response")
		} else {
			t.Errorf("Message body not found in response")
		}
	} else {
		t.Errorf("ReceiveMessageResponse not found in response")
	}
}

func TestReceiveMessageDebug(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := NewSQSHandler(mockStorage, "http://localhost:9324")

	// Create queue
	queue := &storage.Queue{
		Name:      "debug-queue",
		URL:       "http://localhost:9324/debug-queue",
		CreatedAt: time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	// Add message
	message := &storage.Message{
		ID:        "debug-msg",
		QueueName: "debug-queue",
		Body:      "debug message",
		CreatedAt: time.Now(),
	}
	mockStorage.SendMessage(context.Background(), message)

	// Verify the mock storage ReceiveMessages method works directly
	receivedMessages, err := mockStorage.ReceiveMessages(context.Background(), "debug-queue", 1, 0, 60)
	if err != nil {
		t.Fatalf("Direct ReceiveMessages call failed: %v", err)
	}

	t.Logf("Direct call returned %d messages", len(receivedMessages))
	if len(receivedMessages) > 0 {
		t.Logf("Message: ID=%s, Body=%s, VisibilityTimeout=%v",
			receivedMessages[0].ID, receivedMessages[0].Body, receivedMessages[0].VisibilityTimeout)
	}

	// Now test via HTTP handler
	formData := url.Values{}
	formData.Set("Action", "ReceiveMessage")
	formData.Set("QueueUrl", "http://localhost:9324/debug-queue")

	req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	t.Logf("HTTP Response: %d", w.Code)
	t.Logf("HTTP Body: %s", w.Body.String())
}
