package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"sqs-backend/src/api"
	"sqs-backend/src/storage"
)

func TestReceiveMessageVisibilityTimeout(t *testing.T) {
	// Setup
	mockStorage := NewMockStorage()
	handler := api.NewSQSHandler(mockStorage, "http://localhost:9324")

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
	handler := api.NewSQSHandler(mockStorage, "http://localhost:9324")

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
	handler := api.NewSQSHandler(mockStorage, "http://localhost:9324")

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
