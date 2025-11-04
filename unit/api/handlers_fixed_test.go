package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"simple-message-queue/src/api"
	"simple-message-queue/src/storage"
)

func TestReceiveMessageVisibilityTimeoutFixed(t *testing.T) {
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
			mockStorage := NewMockStorage()
			handler := api.NewSMQHandler(mockStorage, "http://localhost:9324", "test_admin", "test_password")

			queue := &storage.Queue{
				Name:                     "test-queue-" + tt.name,
				URL:                      "http://localhost:9324/test-queue-" + tt.name,
				VisibilityTimeoutSeconds: 30,
				CreatedAt:                time.Now(),
			}
			mockStorage.CreateQueue(context.Background(), queue)

			message := &storage.Message{
				ID:            "test-msg-" + tt.name,
				QueueName:     "test-queue-" + tt.name,
				Body:          "test message for " + tt.name,
				ReceiptHandle: "test-receipt-" + tt.name,
				CreatedAt:     time.Now(),
			}
			mockStorage.SendMessage(context.Background(), message)

			formData := url.Values{}
			formData.Set("Action", "ReceiveMessage")
			formData.Set("QueueUrl", "http://localhost:9324/test-queue-"+tt.name)
			if tt.visibilityTimeout != "" {
				formData.Set("VisibilityTimeout", tt.visibilityTimeout)
			}

			req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			w := httptest.NewRecorder()
			callSQSHandler(handler, w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
				return
			}

			if mockStorage.lastVisibilityTimeout != tt.expectedTimeout {
				t.Errorf("%s: Expected visibility timeout %d, got %d",
					tt.description, tt.expectedTimeout, mockStorage.lastVisibilityTimeout)
			}

			responseBody := w.Body.String()
			if !strings.Contains(responseBody, "ReceiveMessageResponse") {
				t.Errorf("Response should contain ReceiveMessageResponse element. Got: %s", responseBody)
			}

			if !strings.Contains(responseBody, tt.name) {
				t.Errorf("Response should contain message body with test identifier. Got: %s", responseBody)
			}
		})
	}
}

func TestReceiveMessageParameterHandlingFixed(t *testing.T) {
	mockStorage := NewMockStorage()
	handler := api.NewSMQHandler(mockStorage, "http://localhost:9324", "test_admin", "test_password")

	queue := &storage.Queue{
		Name:                     "test-params-queue",
		URL:                      "http://localhost:9324/test-params-queue",
		VisibilityTimeoutSeconds: 30,
		CreatedAt:                time.Now(),
	}
	mockStorage.CreateQueue(context.Background(), queue)

	for i := 0; i < 5; i++ {
		message := &storage.Message{
			ID:            "test-param-msg-" + string(rune('0'+i)),
			QueueName:     "test-params-queue",
			Body:          "test message " + string(rune('0'+i)),
			ReceiptHandle: "test-param-receipt-" + string(rune('0'+i)),
			CreatedAt:     time.Now(),
		}
		mockStorage.SendMessage(context.Background(), message)
	}

	t.Run("all_parameters_together", func(t *testing.T) {
		mockStorage.lastVisibilityTimeout = -1

		formData := url.Values{}
		formData.Set("Action", "ReceiveMessage")
		formData.Set("QueueUrl", "http://localhost:9324/test-params-queue")
		formData.Set("MaxNumberOfMessages", "3")
		formData.Set("WaitTimeSeconds", "5")
		formData.Set("VisibilityTimeout", "120")

		req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		callSQSHandler(handler, w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
			return
		}

		if mockStorage.lastVisibilityTimeout != 120 {
			t.Errorf("Expected visibility timeout 120, got %d", mockStorage.lastVisibilityTimeout)
		}

		responseBody := w.Body.String()
		if !strings.Contains(responseBody, "ReceiveMessageResponse") {
			t.Errorf("Response should contain ReceiveMessageResponse element")
		}

		if !strings.Contains(responseBody, "test message") {
			t.Errorf("Response should contain at least one message. Got: %s", responseBody)
		}
	})
}
