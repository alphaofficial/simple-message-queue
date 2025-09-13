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

func TestGetQueueAttributes(t *testing.T) {
	// Setup
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost:9324")

	// Create test queue with custom attributes
	queue := &storage.Queue{
		Name:                          "test-attributes-queue",
		URL:                           "http://localhost:9324/test-attributes-queue",
		VisibilityTimeoutSeconds:      45,
		MessageRetentionPeriod:        604800, // 7 days
		DelaySeconds:                  10,
		ReceiveMessageWaitTimeSeconds: 5,
		RedrivePolicy:                 `{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:123456789012:dlq","maxReceiveCount":3}`,
		CreatedAt:                     time.Unix(1640995200, 0), // Fixed timestamp for testing
	}
	mockStorage.CreateQueue(context.Background(), queue)

	// Add some test messages for count testing
	testMessages := []*storage.Message{
		{
			ID:        "msg-1",
			QueueName: "test-attributes-queue",
			Body:      "Available message 1",
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg-2",
			QueueName: "test-attributes-queue",
			Body:      "Available message 2",
			CreatedAt: time.Now(),
		},
		{
			ID:        "msg-3",
			QueueName: "test-attributes-queue",
			Body:      "In-flight message",
			CreatedAt: time.Now(),
			VisibleAt: timePtr(time.Now().Add(300 * time.Second)), // In flight for 5 minutes
		},
		{
			ID:           "msg-4",
			QueueName:    "test-attributes-queue",
			Body:         "Delayed message",
			DelaySeconds: 60, // Delayed for 1 minute
			CreatedAt:    time.Now(),
		},
	}

	for _, msg := range testMessages {
		mockStorage.SendMessage(context.Background(), msg)
	}

	tests := []struct {
		name               string
		requestedAttrs     []string
		expectedAttrs      map[string]string
		shouldContainAttrs []string
		description        string
	}{
		{
			name:           "all_attributes",
			requestedAttrs: []string{"All"},
			expectedAttrs: map[string]string{
				"VisibilityTimeout":                     "45",
				"MaximumMessageSize":                    "262144",
				"MessageRetentionPeriod":                "604800",
				"DelaySeconds":                          "10",
				"ReceiveMessageWaitTimeSeconds":         "5",
				"ApproximateNumberOfMessages":           "2", // msg-1, msg-2
				"ApproximateNumberOfMessagesNotVisible": "1", // msg-3 (in flight)
				"ApproximateNumberOfMessagesDelayed":    "1", // msg-4 (delayed)
				"CreatedTimestamp":                      "1640995200",
				"LastModifiedTimestamp":                 "1640995200",
				"QueueArn":                              "arn:aws:sqs:us-east-1:123456789012:test-attributes-queue",
				"RedrivePolicy":                         `{&#34;deadLetterTargetArn&#34;:&#34;arn:aws:sqs:us-east-1:123456789012:dlq&#34;,&#34;maxReceiveCount&#34;:3}`,
				"SqsManagedSseEnabled":                  "false",
			},
			description: "Should return all available attributes when 'All' is requested",
		},
		{
			name:           "specific_attributes",
			requestedAttrs: []string{"VisibilityTimeout", "ApproximateNumberOfMessages", "QueueArn"},
			expectedAttrs: map[string]string{
				"VisibilityTimeout":           "45",
				"ApproximateNumberOfMessages": "2",
				"QueueArn":                    "arn:aws:sqs:us-east-1:123456789012:test-attributes-queue",
			},
			description: "Should return only specifically requested attributes",
		},
		{
			name:           "message_count_attributes",
			requestedAttrs: []string{"ApproximateNumberOfMessages", "ApproximateNumberOfMessagesNotVisible", "ApproximateNumberOfMessagesDelayed"},
			expectedAttrs: map[string]string{
				"ApproximateNumberOfMessages":           "2",
				"ApproximateNumberOfMessagesNotVisible": "1",
				"ApproximateNumberOfMessagesDelayed":    "1",
			},
			description: "Should return accurate live message counts",
		},
		{
			name:           "core_operational_attributes",
			requestedAttrs: []string{"VisibilityTimeout", "MaximumMessageSize", "MessageRetentionPeriod", "DelaySeconds"},
			expectedAttrs: map[string]string{
				"VisibilityTimeout":      "45",
				"MaximumMessageSize":     "262144",
				"MessageRetentionPeriod": "604800",
				"DelaySeconds":           "10",
			},
			description: "Should return core operational queue attributes",
		},
		{
			name:               "no_attributes_defaults_to_all",
			requestedAttrs:     []string{},
			shouldContainAttrs: []string{"VisibilityTimeout", "ApproximateNumberOfMessages", "QueueArn"},
			description:        "Should default to 'All' when no specific attributes requested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create form data
			formData := url.Values{}
			formData.Set("Action", "GetQueueAttributes")
			formData.Set("QueueUrl", "http://localhost:9324/test-attributes-queue")

			// Add attribute names if specified
			for i, attr := range tt.requestedAttrs {
				formData.Set(fmt.Sprintf("AttributeName.%d", i+1), attr)
			}

			req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Execute request
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			// Verify response
			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
				return
			}

			responseBody := w.Body.String()

			// Verify XML structure
			if !strings.Contains(responseBody, "GetQueueAttributesResponse") {
				t.Errorf("Response should contain GetQueueAttributesResponse element")
			}

			// Verify specific attributes if provided
			for attrName, expectedValue := range tt.expectedAttrs {
				namePattern := fmt.Sprintf("<Name>%s</Name>", attrName)
				valuePattern := fmt.Sprintf("<Value>%s</Value>", expectedValue)

				if !strings.Contains(responseBody, namePattern) {
					t.Errorf("%s: Response should contain attribute name %s", tt.description, attrName)
				}

				if !strings.Contains(responseBody, valuePattern) {
					t.Errorf("%s: Response should contain attribute value %s for %s. Got response: %s",
						tt.description, expectedValue, attrName, responseBody)
				}
			}

			// Verify shouldContainAttrs if provided
			for _, attrName := range tt.shouldContainAttrs {
				namePattern := fmt.Sprintf("<Name>%s</Name>", attrName)
				if !strings.Contains(responseBody, namePattern) {
					t.Errorf("%s: Response should contain attribute %s", tt.description, attrName)
				}
			}
		})
	}
}

func TestGetQueueAttributesErrors(t *testing.T) {
	// Setup
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost:9324")

	t.Run("missing_queue_url", func(t *testing.T) {
		formData := url.Values{}
		formData.Set("Action", "GetQueueAttributes")
		// Missing QueueUrl parameter

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		// Should return error for missing parameter
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		responseBody := w.Body.String()
		if !strings.Contains(responseBody, "MissingParameter") {
			t.Errorf("Response should contain MissingParameter error")
		}
	})

	t.Run("nonexistent_queue", func(t *testing.T) {
		formData := url.Values{}
		formData.Set("Action", "GetQueueAttributes")
		formData.Set("QueueUrl", "http://localhost:9324/nonexistent-queue")

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		// Should return error for nonexistent queue
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}

		responseBody := w.Body.String()
		if !strings.Contains(responseBody, "QueueDoesNotExist") {
			t.Errorf("Response should contain QueueDoesNotExist error")
		}
	})
}

func TestGetQueueAttributesMessageCounting(t *testing.T) {
	// Setup
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost:9324")

	// Create test queue
	queue := &storage.Queue{
		Name:      "test-counting-queue",
		URL:       "http://localhost:9324/test-counting-queue",
		CreatedAt: time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	// Test different message state scenarios
	tests := []struct {
		name              string
		messages          []*storage.Message
		expectedAvailable int
		expectedInFlight  int
		expectedDelayed   int
		description       string
	}{
		{
			name:              "empty_queue",
			messages:          []*storage.Message{},
			expectedAvailable: 0,
			expectedInFlight:  0,
			expectedDelayed:   0,
			description:       "Should return zeros for empty queue",
		},
		{
			name: "all_available_messages",
			messages: []*storage.Message{
				{ID: "msg-1", QueueName: "test-counting-queue", Body: "Message 1", CreatedAt: time.Now()},
				{ID: "msg-2", QueueName: "test-counting-queue", Body: "Message 2", CreatedAt: time.Now()},
				{ID: "msg-3", QueueName: "test-counting-queue", Body: "Message 3", CreatedAt: time.Now()},
			},
			expectedAvailable: 3,
			expectedInFlight:  0,
			expectedDelayed:   0,
			description:       "Should count all available messages correctly",
		},
		{
			name: "mixed_message_states",
			messages: []*storage.Message{
				{ID: "msg-1", QueueName: "test-counting-queue", Body: "Available 1", CreatedAt: time.Now()},
				{ID: "msg-2", QueueName: "test-counting-queue", Body: "Available 2", CreatedAt: time.Now()},
				{ID: "msg-3", QueueName: "test-counting-queue", Body: "In flight", CreatedAt: time.Now(), VisibleAt: timePtr(time.Now().Add(300 * time.Second))},
				{ID: "msg-4", QueueName: "test-counting-queue", Body: "Delayed", DelaySeconds: 60, CreatedAt: time.Now()},
				{ID: "msg-5", QueueName: "test-counting-queue", Body: "Also in flight", CreatedAt: time.Now(), VisibleAt: timePtr(time.Now().Add(600 * time.Second))},
			},
			expectedAvailable: 2,
			expectedInFlight:  2,
			expectedDelayed:   1,
			description:       "Should correctly categorize mixed message states",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear queue and add test messages
			mockStorage.messages["test-counting-queue"] = []*storage.Message{}
			for _, msg := range tt.messages {
				mockStorage.SendMessage(context.Background(), msg)
			}

			// Request message count attributes
			formData := url.Values{}
			formData.Set("Action", "GetQueueAttributes")
			formData.Set("QueueUrl", "http://localhost:9324/test-counting-queue")
			formData.Set("AttributeName.1", "ApproximateNumberOfMessages")
			formData.Set("AttributeName.2", "ApproximateNumberOfMessagesNotVisible")
			formData.Set("AttributeName.3", "ApproximateNumberOfMessagesDelayed")

			req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
			}

			responseBody := w.Body.String()

			// Verify message counts
			expectedPairs := map[string]int{
				"ApproximateNumberOfMessages":           tt.expectedAvailable,
				"ApproximateNumberOfMessagesNotVisible": tt.expectedInFlight,
				"ApproximateNumberOfMessagesDelayed":    tt.expectedDelayed,
			}

			for attrName, expectedCount := range expectedPairs {
				namePattern := fmt.Sprintf("<Name>%s</Name>", attrName)
				valuePattern := fmt.Sprintf("<Value>%d</Value>", expectedCount)

				if !strings.Contains(responseBody, namePattern) {
					t.Errorf("%s: Response should contain attribute name %s", tt.description, attrName)
				}

				if !strings.Contains(responseBody, valuePattern) {
					t.Errorf("%s: Response should contain count %d for %s. Response: %s",
						tt.description, expectedCount, attrName, responseBody)
				}
			}
		})
	}
}

// Helper function to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}
