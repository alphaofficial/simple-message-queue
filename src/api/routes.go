package api

import (
	"fmt"
	"net/http"
	"strings"
)

// ServeHTTP implements the http.Handler interface and routes requests to appropriate handlers
func (h *SQSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("DEBUG: %s %s\n", r.Method, r.URL.Path)

	// Handle health check
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		h.handleHealthCheck(w, r)
		return
	}

	// Handle dashboard requests
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		h.handleDashboard(w, r)
		return
	}

	// API endpoints for dashboard
	if strings.HasPrefix(r.URL.Path, "/api/") {
		fmt.Printf("DEBUG: Routing to API handler\n")
		h.handleAPIRoutes(w, r)
		return
	}

	// SQS API endpoints (both JSON and XML protocols)
	if r.Method != http.MethodPost {
		h.writeErrorResponse(w, "InvalidAction", "Only POST method is supported", http.StatusMethodNotAllowed)
		return
	}

	// Detect protocol based on Content-Type
	contentType := r.Header.Get("Content-Type")
	isJSONProtocol := strings.Contains(contentType, "application/x-amz-json-1.0")

	if isJSONProtocol {
		h.handleJSONRequest(w, r)
		return
	}

	// Handle traditional form-encoded SQS requests
	if err := r.ParseForm(); err != nil {
		h.writeErrorResponse(w, "InvalidRequest", "Failed to parse form data", http.StatusBadRequest)
		return
	}

	action := r.FormValue("Action")

	switch action {
	case "CreateQueue":
		h.handleCreateQueue(w, r)
	case "DeleteQueue":
		h.handleDeleteQueue(w, r)
	case "ListQueues":
		h.handleListQueues(w, r)
	case "GetQueueUrl":
		h.handleGetQueueUrl(w, r)
	case "SendMessage":
		h.handleSendMessage(w, r)
	case "SendMessageBatch":
		h.handleSendMessageBatch(w, r)
	case "ReceiveMessage":
		h.handleReceiveMessage(w, r)
	case "DeleteMessage":
		h.handleDeleteMessage(w, r)
	case "DeleteMessageBatch":
		h.handleDeleteMessageBatch(w, r)
	case "ChangeMessageVisibility":
		h.handleChangeMessageVisibility(w, r)
	case "ChangeMessageVisibilityBatch":
		h.handleChangeMessageVisibilityBatch(w, r)
	case "GetQueueAttributes":
		h.handleGetQueueAttributes(w, r)
	case "SetQueueAttributes":
		h.handleSetQueueAttributes(w, r)
	case "PurgeQueue":
		h.handlePurgeQueue(w, r)
	default:
		h.writeErrorResponse(w, "InvalidAction", fmt.Sprintf("Unknown action: %s", action), http.StatusBadRequest)
	}
}

// handleJSONRequest routes JSON protocol requests to appropriate handlers
func (h *SQSHandler) handleJSONRequest(w http.ResponseWriter, r *http.Request) {
	// Extract action from x-amz-target header (case-insensitive)
	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		target = r.Header.Get("x-amz-target")
	}
	if target == "" {
		h.writeJSONErrorResponse(w, "InvalidAction", "Missing x-amz-target header", http.StatusBadRequest)
		return
	}

	// Parse action from target (e.g., "AmazonSQS.SendMessage" -> "SendMessage")
	parts := strings.Split(target, ".")
	if len(parts) != 2 || parts[0] != "AmazonSQS" {
		h.writeJSONErrorResponse(w, "InvalidAction", fmt.Sprintf("Invalid target: %s", target), http.StatusBadRequest)
		return
	}
	action := parts[1]

	// Route to appropriate handler based on action
	switch action {
	case "CreateQueue":
		h.handleJSONCreateQueue(w, r)
	case "DeleteQueue":
		h.handleJSONDeleteQueue(w, r)
	case "ListQueues":
		h.handleJSONListQueues(w, r)
	case "GetQueueUrl":
		h.handleJSONGetQueueUrl(w, r)
	case "SendMessage":
		h.handleJSONSendMessage(w, r)
	case "SendMessageBatch":
		h.handleJSONSendMessageBatch(w, r)
	case "ReceiveMessage":
		h.handleJSONReceiveMessage(w, r)
	case "DeleteMessage":
		h.handleJSONDeleteMessage(w, r)
	case "DeleteMessageBatch":
		h.handleJSONDeleteMessageBatch(w, r)
	case "ChangeMessageVisibility":
		h.handleJSONChangeMessageVisibility(w, r)
	case "ChangeMessageVisibilityBatch":
		h.handleJSONChangeMessageVisibilityBatch(w, r)
	case "GetQueueAttributes":
		h.handleJSONGetQueueAttributes(w, r)
	case "SetQueueAttributes":
		h.handleJSONSetQueueAttributes(w, r)
	case "PurgeQueue":
		h.handleJSONPurgeQueue(w, r)
	default:
		h.writeJSONErrorResponse(w, "InvalidAction", fmt.Sprintf("Unsupported action: %s", action), http.StatusBadRequest)
	}
}

// handleAPIRoutes handles dashboard API routes
func (h *SQSHandler) handleAPIRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.URL.Path {
	case "/api/status":
		if r.Method == http.MethodGet {
			h.handleAPIStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/queues":
		if r.Method == http.MethodGet {
			h.handleAPIListQueues(w, r)
		} else if r.Method == http.MethodPost {
			h.handleAPICreateQueue(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages":
		if r.Method == http.MethodPost {
			h.handleAPISendMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages/poll":
		if r.Method == http.MethodPost {
			h.handleAPIPollMessages(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages/send":
		if r.Method == http.MethodPost {
			h.handleAPISendMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		if strings.HasPrefix(r.URL.Path, "/api/queues/") {
			if strings.HasSuffix(r.URL.Path, "/messages") {
				if r.Method == http.MethodGet {
					h.handleAPIGetQueueMessages(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			} else {
				// Handle individual queue operations like DELETE /api/queues/{name}
				if r.Method == http.MethodDelete {
					h.handleAPIDeleteQueue(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			}
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}
}
