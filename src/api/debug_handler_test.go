package api

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sqs-backend/src/storage"
)

func TestHandlerDebugging(t *testing.T) {
	// Create fresh mock storage
	mockStorage := NewMockStorage()
	handler := NewSQSHandler(mockStorage, "http://localhost:9324")

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

	// Test extractQueueNameFromURL method indirectly
	testHandler := NewSQSHandler(mockStorage, "http://localhost:9324")
	testURL := "http://localhost:9324/debug-handler-queue"
	extractedName := testHandler.extractQueueNameFromURL(testURL)
	t.Logf("Extracted queue name: '%s' from URL: '%s'", extractedName, testURL)

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

	// Test a different queue name to see if extraction works
	t.Run("test_queue_name_extraction", func(t *testing.T) {
		testCases := []struct {
			url      string
			expected string
		}{
			{"http://localhost:9324/simple-queue", "simple-queue"},
			{"http://localhost:9324/my-test-queue", "my-test-queue"},
			{"https://sqs.us-east-1.amazonaws.com/123456789012/MyQueue", "MyQueue"},
		}

		for _, tc := range testCases {
			result := testHandler.extractQueueNameFromURL(tc.url)
			t.Logf("URL: %s -> Queue: %s (expected: %s)", tc.url, result, tc.expected)
			if result != tc.expected {
				t.Errorf("Queue name extraction failed for %s. Expected: %s, Got: %s",
					tc.url, tc.expected, result)
			}
		}
	})
}
