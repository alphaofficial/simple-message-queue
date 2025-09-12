package api

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	storage "github.com/sqs-producer/sqs-server/src/storage"
)

type SQSHandler struct {
	storage storage.Storage
	baseURL string
}

func NewSQSHandler(storage storage.Storage, baseURL string) *SQSHandler {
	return &SQSHandler{
		storage: storage,
		baseURL: baseURL,
	}
}

func (h *SQSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle dashboard requests
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		h.handleDashboard(w, r)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/api/status" {
		h.handleAPIStatus(w, r)
		return
	}

	if r.Method != http.MethodPost {
		h.writeErrorResponse(w, "InvalidAction", "Only POST method is supported", http.StatusMethodNotAllowed)
		return
	}

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
	case "ReceiveMessage":
		h.handleReceiveMessage(w, r)
	case "DeleteMessage":
		h.handleDeleteMessage(w, r)
	case "ChangeMessageVisibility":
		h.handleChangeMessageVisibility(w, r)
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

func (h *SQSHandler) handleCreateQueue(w http.ResponseWriter, r *http.Request) {
	queueName := r.FormValue("QueueName")
	if queueName == "" {
		h.writeErrorResponse(w, "MissingParameter", "QueueName is required", http.StatusBadRequest)
		return
	}

	// Parse attributes
	attributes := make(map[string]string)
	for key, values := range r.Form {
		if strings.HasPrefix(key, "Attribute.") && strings.HasSuffix(key, ".Name") {
			attrIndex := strings.TrimPrefix(key, "Attribute.")
			attrIndex = strings.TrimSuffix(attrIndex, ".Name")

			attrName := values[0]
			attrValueKey := fmt.Sprintf("Attribute.%s.Value", attrIndex)
			if attrValues, ok := r.Form[attrValueKey]; ok && len(attrValues) > 0 {
				attributes[attrName] = attrValues[0]
			}
		}
	}

	// Create queue object
	queue := &storage.Queue{
		Name:                          queueName,
		URL:                           fmt.Sprintf("%s/%s", h.baseURL, queueName),
		Attributes:                    attributes,
		VisibilityTimeoutSeconds:      30,
		MessageRetentionPeriod:        1209600, // 14 days
		MaxReceiveCount:               0,
		DelaySeconds:                  0,
		ReceiveMessageWaitTimeSeconds: 0,
		CreatedAt:                     time.Now(),
	}

	// Parse standard attributes
	if val, ok := attributes["VisibilityTimeout"]; ok {
		if timeout, err := strconv.Atoi(val); err == nil {
			queue.VisibilityTimeoutSeconds = timeout
		}
	}
	if val, ok := attributes["MaxReceiveCount"]; ok {
		if count, err := strconv.Atoi(val); err == nil {
			queue.MaxReceiveCount = count
		}
	}
	if val, ok := attributes["RedrivePolicy"]; ok {
		queue.RedrivePolicy = val
	}

	if err := h.storage.CreateQueue(r.Context(), queue); err != nil {
		h.writeErrorResponse(w, "QueueAlreadyExists", "Queue already exists", http.StatusBadRequest)
		return
	}

	response := CreateQueueResponse{
		QueueURL: queue.URL,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleDeleteQueue(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	if err := h.storage.DeleteQueue(r.Context(), queueName); err != nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) handleListQueues(w http.ResponseWriter, r *http.Request) {
	prefix := r.FormValue("QueueNamePrefix")

	queues, err := h.storage.ListQueues(r.Context(), prefix)
	if err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to list queues", http.StatusInternalServerError)
		return
	}

	var queueURLs []string
	for _, queue := range queues {
		queueURLs = append(queueURLs, queue.URL)
	}

	response := ListQueuesResponse{
		QueueURLs: queueURLs,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleGetQueueUrl(w http.ResponseWriter, r *http.Request) {
	queueName := r.FormValue("QueueName")

	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	response := CreateQueueResponse{
		QueueURL: queue.URL,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)
	messageBody := r.FormValue("MessageBody")

	if messageBody == "" {
		h.writeErrorResponse(w, "MissingParameter", "MessageBody is required", http.StatusBadRequest)
		return
	}

	// Get queue to check max receive count for DLQ
	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	// Create message
	message := &storage.Message{
		ID:                uuid.New().String(),
		QueueName:         queueName,
		Body:              messageBody,
		Attributes:        make(map[string]string),
		MessageAttributes: make(map[string]storage.MessageAttribute),
		ReceiptHandle:     uuid.New().String(),
		ReceiveCount:      0,
		MaxReceiveCount:   queue.MaxReceiveCount,
		CreatedAt:         time.Now(),
		DelaySeconds:      0,
	}

	// Calculate MD5
	hash := md5.Sum([]byte(messageBody))
	message.MD5OfBody = hex.EncodeToString(hash[:])

	// Parse delay seconds
	if delayStr := r.FormValue("DelaySeconds"); delayStr != "" {
		if delay, err := strconv.Atoi(delayStr); err == nil {
			message.DelaySeconds = delay
		}
	}

	// Parse message attributes
	for key, values := range r.Form {
		if strings.HasPrefix(key, "MessageAttribute.") && strings.HasSuffix(key, ".Name") {
			attrIndex := strings.TrimPrefix(key, "MessageAttribute.")
			attrIndex = strings.TrimSuffix(attrIndex, ".Name")

			attrName := values[0]
			dataTypeKey := fmt.Sprintf("MessageAttribute.%s.Value.DataType", attrIndex)
			stringValueKey := fmt.Sprintf("MessageAttribute.%s.Value.StringValue", attrIndex)

			attr := storage.MessageAttribute{}
			if dataType, ok := r.Form[dataTypeKey]; ok && len(dataType) > 0 {
				attr.DataType = dataType[0]
			}
			if stringValue, ok := r.Form[stringValueKey]; ok && len(stringValue) > 0 {
				attr.StringValue = stringValue[0]
			}

			message.MessageAttributes[attrName] = attr
		}
	}

	if err := h.storage.SendMessage(r.Context(), message); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to send message", http.StatusInternalServerError)
		return
	}

	response := SendMessageResponse{
		MessageId:              message.ID,
		MD5OfBody:              message.MD5OfBody,
		MD5OfMessageAttributes: message.MD5OfAttributes,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	maxMessages := 1
	if maxStr := r.FormValue("MaxNumberOfMessages"); maxStr != "" {
		if max, err := strconv.Atoi(maxStr); err == nil && max > 0 && max <= 10 {
			maxMessages = max
		}
	}

	waitTimeSeconds := 0
	if waitStr := r.FormValue("WaitTimeSeconds"); waitStr != "" {
		if wait, err := strconv.Atoi(waitStr); err == nil {
			waitTimeSeconds = wait
		}
	}

	messages, err := h.storage.ReceiveMessages(r.Context(), queueName, maxMessages, waitTimeSeconds)
	if err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to receive messages", http.StatusInternalServerError)
		return
	}

	var responseMessages []Message
	for _, msg := range messages {
		responseMessages = append(responseMessages, Message{
			MessageId:         msg.ID,
			ReceiptHandle:     msg.ReceiptHandle,
			MD5OfBody:         msg.MD5OfBody,
			Body:              msg.Body,
			Attributes:        msg.Attributes,
			MessageAttributes: convertMessageAttributes(msg.MessageAttributes),
		})
	}

	response := ReceiveMessageResponse{
		Messages: responseMessages,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)
	receiptHandle := r.FormValue("ReceiptHandle")

	if receiptHandle == "" {
		h.writeErrorResponse(w, "MissingParameter", "ReceiptHandle is required", http.StatusBadRequest)
		return
	}

	if err := h.storage.DeleteMessage(r.Context(), queueName, receiptHandle); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to delete message", http.StatusInternalServerError)
		return
	}

	response := DeleteMessageResponse{
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleChangeMessageVisibility(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)
	receiptHandle := r.FormValue("ReceiptHandle")
	visibilityTimeoutStr := r.FormValue("VisibilityTimeout")

	visibilityTimeout, err := strconv.Atoi(visibilityTimeoutStr)
	if err != nil {
		h.writeErrorResponse(w, "InvalidParameterValue", "Invalid VisibilityTimeout", http.StatusBadRequest)
		return
	}

	if err := h.storage.ChangeMessageVisibility(r.Context(), queueName, receiptHandle, visibilityTimeout); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to change message visibility", http.StatusInternalServerError)
		return
	}

	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) handleGetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	// Simplified implementation - would need to be expanded
	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) handleSetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	// Simplified implementation - would need to be expanded
	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) handlePurgeQueue(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	if err := h.storage.PurgeQueue(r.Context(), queueName); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to purge queue", http.StatusInternalServerError)
		return
	}

	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) extractQueueNameFromURL(queueURL string) string {
	if queueURL == "" {
		return ""
	}

	u, err := url.Parse(queueURL)
	if err != nil {
		return ""
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ""
}

func (h *SQSHandler) writeErrorResponse(w http.ResponseWriter, code, message string, statusCode int) {
	errorResp := ErrorResponse{
		Error: Error{
			Type:    "Sender",
			Code:    code,
			Message: message,
		},
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)
	xml.NewEncoder(w).Encode(errorResp)
}

func (h *SQSHandler) writeXMLResponse(w http.ResponseWriter, response interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)

	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
	xml.NewEncoder(w).Encode(response)
}

func convertMessageAttributes(attrs map[string]storage.MessageAttribute) map[string]MessageAttribute {
	result := make(map[string]MessageAttribute)
	for k, v := range attrs {
		result[k] = MessageAttribute{
			DataType:    v.DataType,
			StringValue: v.StringValue,
			BinaryValue: v.BinaryValue,
		}
	}
	return result
}

func (h *SQSHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	// Read the dashboard HTML template
	html, err := os.ReadFile("templates/dashboard.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error loading dashboard template"))
		return
	}

	w.Write(html)
}

func (h *SQSHandler) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	// Get all queues
	queues, err := h.storage.ListQueues(r.Context(), "")
	if err != nil {
		http.Error(w, "Failed to fetch queues", http.StatusInternalServerError)
		return
	}

	totalMessages := 0
	totalMessagesInFlight := 0
	var queueStats []map[string]interface{}

	// Get statistics for each queue
	for _, queue := range queues {
		// Get messages for this queue
		messages, err := h.storage.GetInFlightMessages(r.Context(), queue.Name)
		if err != nil {
			continue
		}

		availableCount := 0
		inFlightCount := 0
		var queueInFlightMessages []map[string]interface{}

		for _, msg := range messages {
			totalMessages++
			if msg.VisibleAt != nil && msg.VisibleAt.After(time.Now()) {
				inFlightCount++
				totalMessagesInFlight++

				// Add to queue's in-flight messages list
				queueInFlightMessages = append(queueInFlightMessages, map[string]interface{}{
					"id":           msg.ID,
					"body":         msg.Body,
					"receiveCount": msg.ReceiveCount,
					"createdAt":    msg.CreatedAt,
				})
			} else {
				availableCount++
			}
		}

		queueStats = append(queueStats, map[string]interface{}{
			"name":              queue.Name,
			"availableMessages": availableCount,
			"inFlightMessages":  inFlightCount,
			"totalMessages":     availableCount + inFlightCount,
			"messages":          queueInFlightMessages,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Simple JSON encoding without importing encoding/json
	jsonStr := fmt.Sprintf(`{
		"totalQueues": %d,
		"totalMessages": %d,
		"messagesInFlight": %d,
		"queues": [`, len(queues), totalMessages, totalMessagesInFlight)

	for i, queue := range queueStats {
		if i > 0 {
			jsonStr += ","
		}
		jsonStr += fmt.Sprintf(`{
			"name": "%s",
			"availableMessages": %v,
			"inFlightMessages": %v,
			"totalMessages": %v,
			"messages": [`, queue["name"], queue["availableMessages"], queue["inFlightMessages"], queue["totalMessages"])

		// Add messages for this queue
		if msgs, ok := queue["messages"].([]map[string]interface{}); ok {
			for j, msg := range msgs {
				if j > 0 {
					jsonStr += ","
				}
				jsonStr += fmt.Sprintf(`{
					"id": "%s",
					"body": "%s",
					"receiveCount": %v,
					"createdAt": "%s"
				}`, msg["id"], strings.ReplaceAll(fmt.Sprintf("%v", msg["body"]), `"`, `\"`), msg["receiveCount"], msg["createdAt"])
			}
		}
		jsonStr += "]}"
	}

	jsonStr += fmt.Sprintf(`],
		"timestamp": "%s"
	}`, time.Now().Format(time.RFC3339))

	w.Write([]byte(jsonStr))
}
