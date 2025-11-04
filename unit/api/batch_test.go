package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"simple-message-queue/src/api"
	"simple-message-queue/src/storage"

	"github.com/gin-gonic/gin"
)

func callSQSHandler(handler *api.SMQHandler, w http.ResponseWriter, r *http.Request) {
	router := gin.New()
	handler.SetupRoutes(router)
	router.ServeHTTP(w, r)
}

func TestSendMessageBatch(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSMQHandler(mockStorage, "http://localhost:9324", "", "")

	queue := &storage.Queue{
		Name:      "test-batch-queue",
		URL:       "http://localhost:9324/test-batch-queue",
		CreatedAt: time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	tests := []struct {
		name          string
		entries       []BatchMessage
		expectedCount int
		description   string
	}{
		{
			name: "single_message_batch",
			entries: []BatchMessage{
				{Id: "msg1", Body: "Test message 1"},
			},
			expectedCount: 1,
			description:   "Should handle single message in batch",
		},
		{
			name: "multiple_messages_batch",
			entries: []BatchMessage{
				{Id: "msg1", Body: "Test message 1"},
				{Id: "msg2", Body: "Test message 2"},
				{Id: "msg3", Body: "Test message 3"},
			},
			expectedCount: 3,
			description:   "Should handle multiple messages in batch",
		},
		{
			name: "batch_with_delay",
			entries: []BatchMessage{
				{Id: "msg1", Body: "Test message 1", DelaySeconds: 30},
				{Id: "msg2", Body: "Test message 2", DelaySeconds: 60},
			},
			expectedCount: 2,
			description:   "Should handle DelaySeconds parameter in batch",
		},
		{
			name: "max_batch_size",
			entries: []BatchMessage{
				{Id: "msg1", Body: "Message 1"},
				{Id: "msg2", Body: "Message 2"},
				{Id: "msg3", Body: "Message 3"},
				{Id: "msg4", Body: "Message 4"},
				{Id: "msg5", Body: "Message 5"},
				{Id: "msg6", Body: "Message 6"},
				{Id: "msg7", Body: "Message 7"},
				{Id: "msg8", Body: "Message 8"},
				{Id: "msg9", Body: "Message 9"},
				{Id: "msg10", Body: "Message 10"},
			},
			expectedCount: 10,
			description:   "Should handle maximum batch size (10 messages)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage.messages["test-batch-queue"] = []*storage.Message{}
			formData := url.Values{}
			formData.Set("Action", "SendMessageBatch")
			formData.Set("QueueUrl", "http://localhost:9324/test-batch-queue")

			for i, entry := range tt.entries {
				entryNum := i + 1
				formData.Set(fmt.Sprintf("Entry.%d.Id", entryNum), entry.Id)
				formData.Set(fmt.Sprintf("Entry.%d.MessageBody", entryNum), entry.Body)
				if entry.DelaySeconds > 0 {
					formData.Set(fmt.Sprintf("Entry.%d.DelaySeconds", entryNum), strconv.Itoa(entry.DelaySeconds))
				}
			}

			req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			w := httptest.NewRecorder()
			callSQSHandler(handler, w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
				return
			}

			messages := mockStorage.messages["test-batch-queue"]
			if len(messages) != tt.expectedCount {
				t.Errorf("%s: Expected %d messages, got %d", tt.description, tt.expectedCount, len(messages))
			}

			responseBody := w.Body.String()
			if !strings.Contains(responseBody, "SendMessageBatchResponse") {
				t.Errorf("Response should contain SendMessageBatchResponse element")
			}

			for i, entry := range tt.entries {
				if i >= len(messages) {
					continue
				}
				msg := messages[i]

				if msg.Body != entry.Body {
					t.Errorf("Message %d body mismatch. Expected: %s, Got: %s", i, entry.Body, msg.Body)
				}

				if msg.DelaySeconds != entry.DelaySeconds {
					t.Errorf("Message %d delay mismatch. Expected: %d, Got: %d", i, entry.DelaySeconds, msg.DelaySeconds)
				}

				if msg.MD5OfBody == "" {
					t.Errorf("Message %d should have MD5OfBody calculated", i)
				}
			}
		})
	}
}

func TestDeleteMessageBatch(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSMQHandler(mockStorage, "http://localhost:9324", "", "")

	queue := &storage.Queue{
		Name:      "test-delete-batch-queue",
		URL:       "http://localhost:9324/test-delete-batch-queue",
		CreatedAt: time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	testMessages := []*storage.Message{
		{
			ID:            "msg-1",
			QueueName:     "test-delete-batch-queue",
			Body:          "Message 1",
			ReceiptHandle: "receipt-1",
			CreatedAt:     time.Now(),
		},
		{
			ID:            "msg-2",
			QueueName:     "test-delete-batch-queue",
			Body:          "Message 2",
			ReceiptHandle: "receipt-2",
			CreatedAt:     time.Now(),
		},
		{
			ID:            "msg-3",
			QueueName:     "test-delete-batch-queue",
			Body:          "Message 3",
			ReceiptHandle: "receipt-3",
			CreatedAt:     time.Now(),
		},
	}

	for _, msg := range testMessages {
		mockStorage.SendMessage(context.Background(), msg)
	}

	t.Run("delete_multiple_messages", func(t *testing.T) {
		initialCount := len(mockStorage.messages["test-delete-batch-queue"])
		if initialCount != 3 {
			t.Fatalf("Expected 3 initial messages, got %d", initialCount)
		}

		formData := url.Values{}
		formData.Set("Action", "DeleteMessageBatch")
		formData.Set("QueueUrl", "http://localhost:9324/test-delete-batch-queue")

		formData.Set("Entry.1.Id", "batch-delete-1")
		formData.Set("Entry.1.ReceiptHandle", "receipt-1")
		formData.Set("Entry.2.Id", "batch-delete-2")
		formData.Set("Entry.2.ReceiptHandle", "receipt-2")

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		callSQSHandler(handler, w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		remainingMessages := mockStorage.messages["test-delete-batch-queue"]
		if len(remainingMessages) != 1 {
			t.Errorf("Expected 1 remaining message, got %d", len(remainingMessages))
		}

		if len(remainingMessages) > 0 && remainingMessages[0].ReceiptHandle != "receipt-3" {
			t.Errorf("Wrong message remained. Expected receipt-3, got %s", remainingMessages[0].ReceiptHandle)
		}

		responseBody := w.Body.String()
		if !strings.Contains(responseBody, "DeleteMessageBatchResponse") {
			t.Errorf("Response should contain DeleteMessageBatchResponse element")
		}
	})
}

func TestChangeMessageVisibilityBatch(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSMQHandler(mockStorage, "http://localhost:9324", "", "")

	queue := &storage.Queue{
		Name:      "test-visibility-batch-queue",
		URL:       "http://localhost:9324/test-visibility-batch-queue",
		CreatedAt: time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	t.Run("change_multiple_message_visibility", func(t *testing.T) {
		formData := url.Values{}
		formData.Set("Action", "ChangeMessageVisibilityBatch")
		formData.Set("QueueUrl", "http://localhost:9324/test-visibility-batch-queue")

		formData.Set("Entry.1.Id", "vis-1")
		formData.Set("Entry.1.ReceiptHandle", "receipt-handle-1")
		formData.Set("Entry.1.VisibilityTimeout", "300")

		formData.Set("Entry.2.Id", "vis-2")
		formData.Set("Entry.2.ReceiptHandle", "receipt-handle-2")
		formData.Set("Entry.2.VisibilityTimeout", "600")

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		callSQSHandler(handler, w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		responseBody := w.Body.String()
		if !strings.Contains(responseBody, "ChangeMessageVisibilityBatchResponse") {
			t.Errorf("Response should contain ChangeMessageVisibilityBatchResponse element")
		}
	})

	t.Run("invalid_visibility_timeout_ignored", func(t *testing.T) {
		formData := url.Values{}
		formData.Set("Action", "ChangeMessageVisibilityBatch")
		formData.Set("QueueUrl", "http://localhost:9324/test-visibility-batch-queue")

		formData.Set("Entry.1.Id", "vis-1")
		formData.Set("Entry.1.ReceiptHandle", "receipt-handle-1")
		formData.Set("Entry.1.VisibilityTimeout", "invalid")

		formData.Set("Entry.2.Id", "vis-2")
		formData.Set("Entry.2.ReceiptHandle", "receipt-handle-2")
		formData.Set("Entry.2.VisibilityTimeout", "300")

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		callSQSHandler(handler, w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})
}

func TestBatchOperationErrors(t *testing.T) {
	// Setup
	mockStorage := NewMockStorage()
	handler := api.NewSMQHandler(mockStorage, "http://localhost:9324", "", "")

	// Create test queue
	queue := &storage.Queue{
		Name:      "test-error-queue",
		URL:       "http://localhost:9324/test-error-queue",
		CreatedAt: time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	t.Run("send_batch_empty_entries", func(t *testing.T) {
		formData := url.Values{}
		formData.Set("Action", "SendMessageBatch")
		formData.Set("QueueUrl", "http://localhost:9324/test-error-queue")

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		callSQSHandler(handler, w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for empty batch, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("delete_batch_empty_entries", func(t *testing.T) {
		formData := url.Values{}
		formData.Set("Action", "DeleteMessageBatch")
		formData.Set("QueueUrl", "http://localhost:9324/test-error-queue")

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		callSQSHandler(handler, w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for empty batch, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

type BatchMessage struct {
	Id           string
	Body         string
	DelaySeconds int
}
