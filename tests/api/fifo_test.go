package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sqs-bridge/src/api"
	"sqs-bridge/src/storage"
)

func TestFifoQueueCreation(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost")

	tests := []struct {
		name       string
		queueName  string
		attributes map[string]string
		expectErr  bool
		errMsg     string
	}{
		{
			name:      "Valid FIFO queue with default attributes",
			queueName: "test-queue.fifo",
			attributes: map[string]string{
				"FifoQueue": "true",
			},
			expectErr: false,
		},
		{
			name:      "Valid FIFO queue with ContentBasedDeduplication",
			queueName: "test-content-dedup.fifo",
			attributes: map[string]string{
				"FifoQueue":                 "true",
				"ContentBasedDeduplication": "true",
				"DeduplicationScope":        "messageGroup",
				"FifoThroughputLimit":       "perMessageGroupId",
			},
			expectErr: false,
		},
		{
			name:      "Invalid FIFO queue without .fifo suffix",
			queueName: "test-queue",
			attributes: map[string]string{
				"FifoQueue": "true",
			},
			expectErr: true,
			errMsg:    "FIFO queue name must end with .fifo suffix",
		},
		{
			name:      "FIFO queue with invalid DelaySeconds",
			queueName: "test-delay.fifo",
			attributes: map[string]string{
				"FifoQueue":    "true",
				"DelaySeconds": "10",
			},
			expectErr: true,
			errMsg:    "FIFO queues do not support DelaySeconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formData := url.Values{}
			formData.Set("Action", "CreateQueue")
			formData.Set("QueueName", tt.queueName)

			index := 1
			for key, value := range tt.attributes {
				formData.Set(fmt.Sprintf("Attribute.%d.Name", index), key)
				formData.Set(fmt.Sprintf("Attribute.%d.Value", index), value)
				index++
			}

			req, _ := http.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.expectErr {
				if rr.Code == http.StatusOK {
					t.Errorf("Expected error but got success")
				}
				if !strings.Contains(rr.Body.String(), tt.errMsg) {
					t.Errorf("Expected error message '%s' but got '%s'", tt.errMsg, rr.Body.String())
				}
			} else {
				if rr.Code != http.StatusOK {
					t.Errorf("Expected success but got error: %s", rr.Body.String())
				}

				// Verify queue was created with correct FIFO attributes
				queue, err := mockStorage.GetQueue(context.Background(), tt.queueName)
				if err != nil || queue == nil {
					t.Errorf("Queue was not created properly")
				} else {
					if !queue.FifoQueue {
						t.Errorf("Queue should be marked as FIFO")
					}
					if val, ok := tt.attributes["ContentBasedDeduplication"]; ok && val == "true" {
						if !queue.ContentBasedDeduplication {
							t.Errorf("ContentBasedDeduplication should be enabled")
						}
					}
				}
			}
		})
	}
}

func TestFifoMessageSending(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost")

	// Create FIFO queue first
	fifoQueue := &storage.Queue{
		Name:                      "test-queue.fifo",
		URL:                       "http://localhost/test-queue.fifo",
		FifoQueue:                 true,
		ContentBasedDeduplication: false,
		DeduplicationScope:        "queue",
		FifoThroughputLimit:       "perQueue",
		CreatedAt:                 time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), fifoQueue)

	tests := []struct {
		name           string
		messageBody    string
		messageGroupId string
		dedupId        string
		expectErr      bool
		errMsg         string
	}{
		{
			name:           "Valid FIFO message",
			messageBody:    "Hello World",
			messageGroupId: "group1",
			dedupId:        "dedup1",
			expectErr:      false,
		},
		{
			name:           "Missing MessageGroupId",
			messageBody:    "Hello World",
			messageGroupId: "",
			dedupId:        "dedup2",
			expectErr:      true,
			errMsg:         "MessageGroupId is required for FIFO queues",
		},
		{
			name:           "Missing MessageDeduplicationId (when ContentBasedDeduplication is disabled)",
			messageBody:    "Hello World",
			messageGroupId: "group1",
			dedupId:        "",
			expectErr:      true,
			errMsg:         "MessageDeduplicationId is required when ContentBasedDeduplication is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formData := url.Values{}
			formData.Set("Action", "SendMessage")
			formData.Set("QueueUrl", fifoQueue.URL)
			formData.Set("MessageBody", tt.messageBody)

			if tt.messageGroupId != "" {
				formData.Set("MessageGroupId", tt.messageGroupId)
			}
			if tt.dedupId != "" {
				formData.Set("MessageDeduplicationId", tt.dedupId)
			}

			req, _ := http.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.expectErr {
				if rr.Code == http.StatusOK {
					t.Errorf("Expected error but got success")
				}
				if !strings.Contains(rr.Body.String(), tt.errMsg) {
					t.Errorf("Expected error message '%s' but got '%s'", tt.errMsg, rr.Body.String())
				}
			} else {
				if rr.Code != http.StatusOK {
					t.Errorf("Expected success but got error: %s", rr.Body.String())
				}

				// Verify message was stored with FIFO fields
				messages := mockStorage.messages[fifoQueue.Name]
				if len(messages) == 0 {
					t.Errorf("Message was not stored")
				} else {
					msg := messages[len(messages)-1]
					if msg.MessageGroupId != tt.messageGroupId {
						t.Errorf("MessageGroupId mismatch: expected %s, got %s", tt.messageGroupId, msg.MessageGroupId)
					}
					if msg.MessageDeduplicationId != tt.dedupId {
						t.Errorf("MessageDeduplicationId mismatch: expected %s, got %s", tt.dedupId, msg.MessageDeduplicationId)
					}
					if msg.SequenceNumber == "" {
						t.Errorf("SequenceNumber should be generated")
					}
				}
			}
		})
	}
}

func TestFifoContentBasedDeduplication(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost")

	// Create FIFO queue with ContentBasedDeduplication enabled
	fifoQueue := &storage.Queue{
		Name:                      "test-content-dedup.fifo",
		URL:                       "http://localhost/test-content-dedup.fifo",
		FifoQueue:                 true,
		ContentBasedDeduplication: true,
		DeduplicationScope:        "queue",
		FifoThroughputLimit:       "perQueue",
		CreatedAt:                 time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), fifoQueue)

	// Send first message
	formData := url.Values{}
	formData.Set("Action", "SendMessage")
	formData.Set("QueueUrl", fifoQueue.URL)
	formData.Set("MessageBody", "Hello World")
	formData.Set("MessageGroupId", "group1")

	req, _ := http.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("First message should succeed: %s", rr.Body.String())
	}

	// Verify message was stored and has generated deduplication ID
	messages := mockStorage.messages[fifoQueue.Name]
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	} else {
		msg := messages[0]
		if msg.MessageDeduplicationId == "" {
			t.Errorf("MessageDeduplicationId should be auto-generated for content-based deduplication")
		}
		if msg.DeduplicationHash == "" {
			t.Errorf("DeduplicationHash should be generated")
		}
	}
}

func TestFifoMessageOrdering(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost")

	// Create FIFO queue
	fifoQueue := &storage.Queue{
		Name:                      "test-ordering.fifo",
		URL:                       "http://localhost/test-ordering.fifo",
		FifoQueue:                 true,
		ContentBasedDeduplication: false,
		DeduplicationScope:        "queue",
		FifoThroughputLimit:       "perQueue",
		CreatedAt:                 time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), fifoQueue)

	// Send messages in order
	messageGroup := "group1"
	for i := 1; i <= 5; i++ {
		formData := url.Values{}
		formData.Set("Action", "SendMessage")
		formData.Set("QueueUrl", fifoQueue.URL)
		formData.Set("MessageBody", fmt.Sprintf("Message %d", i))
		formData.Set("MessageGroupId", messageGroup)
		formData.Set("MessageDeduplicationId", fmt.Sprintf("dedup%d", i))

		req, _ := http.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Message %d should succeed: %s", i, rr.Body.String())
		}

		// Small delay to ensure different sequence numbers
		time.Sleep(1 * time.Millisecond)
	}

	// Verify messages are stored in order
	messages := mockStorage.messages[fifoQueue.Name]
	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}

	// Check sequence numbers are monotonically increasing
	for i := 1; i < len(messages); i++ {
		prev := messages[i-1].SequenceNumber
		curr := messages[i].SequenceNumber
		if prev >= curr {
			t.Errorf("Sequence numbers should be increasing: %s >= %s", prev, curr)
		}
	}
}

func TestFifoQueueAttributes(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost")

	// Create FIFO queue with specific attributes
	fifoQueue := &storage.Queue{
		Name:                      "test-attributes.fifo",
		URL:                       "http://localhost/test-attributes.fifo",
		FifoQueue:                 true,
		ContentBasedDeduplication: true,
		DeduplicationScope:        "messageGroup",
		FifoThroughputLimit:       "perMessageGroupId",
		CreatedAt:                 time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), fifoQueue)

	// Get queue attributes
	formData := url.Values{}
	formData.Set("Action", "GetQueueAttributes")
	formData.Set("QueueUrl", fifoQueue.URL)
	formData.Set("AttributeName.1", "All")

	req, _ := http.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GetQueueAttributes should succeed: %s", rr.Body.String())
	}

	responseBody := rr.Body.String()

	// Check for FIFO-specific attributes in response
	expectedAttributes := []string{
		"FifoQueue",
		"ContentBasedDeduplication",
		"DeduplicationScope",
		"FifoThroughputLimit",
	}

	for _, attr := range expectedAttributes {
		if !strings.Contains(responseBody, attr) {
			t.Errorf("Response should contain FIFO attribute: %s", attr)
		}
	}

	// Check specific values
	if !strings.Contains(responseBody, "<Value>true</Value>") {
		t.Errorf("FifoQueue should be true")
	}
	if !strings.Contains(responseBody, "messageGroup") {
		t.Errorf("DeduplicationScope should be messageGroup")
	}
	if !strings.Contains(responseBody, "perMessageGroupId") {
		t.Errorf("FifoThroughputLimit should be perMessageGroupId")
	}
}
