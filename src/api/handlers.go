package api

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	storage "simple-message-queue/src/storage"
)

func calculateMD5OfMessageAttributes(attrs map[string]storage.MessageAttribute) string {
	if len(attrs) == 0 {
		return ""
	}

	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)

	// Build the buffer according to AWS spec
	var buffer []byte

	for _, name := range names {
		attr := attrs[name]

		nameBytes := []byte(name)
		nameLenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(nameLenBytes, uint32(len(nameBytes)))
		buffer = append(buffer, nameLenBytes...)
		buffer = append(buffer, nameBytes...)

		dataTypeBytes := []byte(attr.DataType)
		dataTypeLenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(dataTypeLenBytes, uint32(len(dataTypeBytes)))
		buffer = append(buffer, dataTypeLenBytes...)
		buffer = append(buffer, dataTypeBytes...)

		if strings.HasPrefix(attr.DataType, "Binary") {
			buffer = append(buffer, 2)
			// For binary, encode the raw bytes
			valueLenBytes := make([]byte, 4)
			binary.BigEndian.PutUint32(valueLenBytes, uint32(len(attr.BinaryValue)))
			buffer = append(buffer, valueLenBytes...)
			buffer = append(buffer, attr.BinaryValue...)
		} else {
			buffer = append(buffer, 1)
			valueBytes := []byte(attr.StringValue)
			valueLenBytes := make([]byte, 4)
			binary.BigEndian.PutUint32(valueLenBytes, uint32(len(valueBytes)))
			buffer = append(buffer, valueLenBytes...)
			buffer = append(buffer, valueBytes...)
		}
	}

	hash := md5.Sum(buffer)
	return hex.EncodeToString(hash[:])
}

type SMQHandler struct {
	storage       storage.Storage
	baseURL       string
	adminUsername string
	adminPassword string
}

func NewSMQHandler(storage storage.Storage, baseURL, adminUsername, adminPassword string) *SMQHandler {
	return &SMQHandler{
		storage:       storage,
		baseURL:       baseURL,
		adminUsername: adminUsername,
		adminPassword: adminPassword,
	}
}

func (h *SMQHandler) handleCreateQueue(w http.ResponseWriter, r *http.Request) {
	queueName := r.FormValue("QueueName")
	if queueName == "" {
		h.writeErrorResponse(w, "MissingParameter", "QueueName is required", http.StatusBadRequest)
		return
	}

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

	if val, ok := attributes["VisibilityTimeout"]; ok {
		if timeout, err := strconv.Atoi(val); err == nil {
			queue.VisibilityTimeoutSeconds = timeout
		}
	}
	if val, ok := attributes["MessageRetentionPeriod"]; ok {
		if period, err := strconv.Atoi(val); err == nil {
			queue.MessageRetentionPeriod = period
		}
	}
	if val, ok := attributes["MaxReceiveCount"]; ok {
		if count, err := strconv.Atoi(val); err == nil {
			queue.MaxReceiveCount = count
		}
	}
	if val, ok := attributes["DelaySeconds"]; ok {
		if delay, err := strconv.Atoi(val); err == nil {
			queue.DelaySeconds = delay
		}
	}
	if val, ok := attributes["RedrivePolicy"]; ok {
		queue.RedrivePolicy = val
		// Parse RedrivePolicy JSON to extract DLQ settings
		var redrivePolicy struct {
			DeadLetterTargetArn string `json:"deadLetterTargetArn"`
			MaxReceiveCount     int    `json:"maxReceiveCount"`
		}
		if err := json.Unmarshal([]byte(val), &redrivePolicy); err == nil {
			// Extract queue name from ARN (format: arn:aws:sqs:region:account:queuename)
			parts := strings.Split(redrivePolicy.DeadLetterTargetArn, ":")
			if len(parts) >= 6 {
				queue.DeadLetterQueueName = parts[5]
				queue.MaxReceiveCount = redrivePolicy.MaxReceiveCount
			}
		}
	}

	if val, ok := attributes["ContentBasedDeduplication"]; ok {
		queue.ContentBasedDeduplication = (val == "true")
	}
	if val, ok := attributes["DeduplicationScope"]; ok {
		queue.DeduplicationScope = val
	}
	if val, ok := attributes["FifoThroughputLimit"]; ok {
		queue.FifoThroughputLimit = val
	}

	if err := SetupFifoQueue(queue); err != nil {
		h.writeErrorResponse(w, "InvalidParameterValue", err.Error(), http.StatusBadRequest)
		return
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

func (h *SMQHandler) handleDeleteQueue(w http.ResponseWriter, r *http.Request) {
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

func (h *SMQHandler) handleListQueues(w http.ResponseWriter, r *http.Request) {
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

func (h *SMQHandler) handleGetQueueUrl(w http.ResponseWriter, r *http.Request) {
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

func (h *SMQHandler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)
	messageBody := r.FormValue("MessageBody")

	if messageBody == "" {
		h.writeErrorResponse(w, "MissingParameter", "MessageBody is required", http.StatusBadRequest)
		return
	}

	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

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

	hash := md5.Sum([]byte(messageBody))
	message.MD5OfBody = hex.EncodeToString(hash[:])

	if delayStr := r.FormValue("DelaySeconds"); delayStr != "" {
		if delay, err := strconv.Atoi(delayStr); err == nil {
			message.DelaySeconds = delay
		}
	}

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

		message.MD5OfAttributes = calculateMD5OfMessageAttributes(message.MessageAttributes)
	}

	message.MessageGroupId = r.FormValue("MessageGroupId")
	message.MessageDeduplicationId = r.FormValue("MessageDeduplicationId")

	if err := ValidateFifoMessage(message, queue); err != nil {
		h.writeErrorResponse(w, "InvalidParameterValue", err.Error(), http.StatusBadRequest)
		return
	}

	if err := PrepareFifoMessage(message, queue); err != nil {
		h.writeErrorResponse(w, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	if queue.FifoQueue && message.DeduplicationHash != "" {
		isDuplicate, err := CheckForDuplicateMessage(r.Context(), h.storage, queueName, message.DeduplicationHash)
		if err != nil {
			h.writeErrorResponse(w, "InternalError", "Failed to check for duplicates", http.StatusInternalServerError)
			return
		}

		if isDuplicate {
			h.writeXMLResponse(w, SendMessageResponse{
				MessageId:              message.ID,
				MD5OfBody:              message.MD5OfBody,
				MD5OfMessageAttributes: message.MD5OfAttributes,
				MessageDeduplicationId: message.MessageDeduplicationId,
				MessageGroupId:         message.MessageGroupId,
				SequenceNumber:         message.SequenceNumber,
				SQSResponse: SQSResponse{
					RequestId: uuid.New().String(),
				},
			}, http.StatusOK)
			return
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
		MessageDeduplicationId: message.MessageDeduplicationId,
		MessageGroupId:         message.MessageGroupId,
		SequenceNumber:         message.SequenceNumber,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SMQHandler) handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
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

	visibilityTimeout := 0 // 0 means use queue default
	if visStr := r.FormValue("VisibilityTimeout"); visStr != "" {
		if vis, err := strconv.Atoi(visStr); err == nil && vis >= 0 && vis <= 43200 { // AWS max is 12 hours
			visibilityTimeout = vis
		}
	}

	messages, err := h.storage.ReceiveMessages(r.Context(), queueName, maxMessages, waitTimeSeconds, visibilityTimeout)
	if err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to receive messages", http.StatusInternalServerError)
		return
	}

	var responseMessages []Message
	for _, msg := range messages {
		responseMessages = append(responseMessages, Message{
			MessageId:              msg.ID,
			ReceiptHandle:          msg.ReceiptHandle,
			MD5OfBody:              msg.MD5OfBody, // Legacy field for backward compatibility
			MD5OfMessageBody:       msg.MD5OfBody, // Modern field (same value for compatibility)
			MD5OfMessageAttributes: msg.MD5OfAttributes,
			Body:                   msg.Body,
			Attributes:             convertAttributes(msg.Attributes),
			MessageAttributes:      convertMessageAttributesToSlice(msg.MessageAttributes),
			MessageGroupId:         msg.MessageGroupId,
			MessageDeduplicationId: msg.MessageDeduplicationId,
			SequenceNumber:         msg.SequenceNumber,
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

func (h *SMQHandler) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
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

func (h *SMQHandler) handleChangeMessageVisibility(w http.ResponseWriter, r *http.Request) {
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

func (h *SMQHandler) handleGetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	if queueName == "" {
		h.writeErrorResponse(w, "MissingParameter", "QueueUrl is required", http.StatusBadRequest)
		return
	}

	// Get queue information
	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	var requestedAttributes []string
	attributeIndex := 1
	for {
		attrKey := fmt.Sprintf("AttributeName.%d", attributeIndex)
		attrName := r.FormValue(attrKey)
		if attrName == "" {
			break
		}
		requestedAttributes = append(requestedAttributes, attrName)
		attributeIndex++
	}

	if len(requestedAttributes) == 0 {
		requestedAttributes = []string{"All"}
	}

	// Generate attributes
	attributes, err := h.generateQueueAttributes(r.Context(), queue, requestedAttributes)
	if err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to generate queue attributes", http.StatusInternalServerError)
		return
	}

	response := GetQueueAttributesResponse{
		Attributes: attributes,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SMQHandler) generateQueueAttributes(ctx context.Context, queue *storage.Queue, requestedAttrs []string) ([]QueueAttribute, error) {
	var attributes []QueueAttribute

	// Check if "All" is requested
	includeAll := false
	attrSet := make(map[string]bool)
	for _, attr := range requestedAttrs {
		if attr == "All" {
			includeAll = true
		}
		attrSet[attr] = true
	}

	shouldInclude := func(attrName string) bool {
		return includeAll || attrSet[attrName]
	}

	// Get live message counts
	allMessages, err := h.storage.GetInFlightMessages(ctx, queue.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages for metrics: %w", err)
	}

	now := time.Now()
	var availableCount, inFlightCount, delayedCount int

	for _, msg := range allMessages {
		// Check if message is delayed
		if msg.DelaySeconds > 0 {
			delayTime := msg.CreatedAt.Add(time.Duration(msg.DelaySeconds) * time.Second)
			if now.Before(delayTime) {
				delayedCount++
				continue
			}
		}

		// Check if message is in flight (invisible)
		if msg.VisibleAt != nil && now.Before(*msg.VisibleAt) {
			inFlightCount++
		} else {
			availableCount++
		}
	}

	// Core operational attributes
	if shouldInclude("VisibilityTimeout") {
		attributes = append(attributes, QueueAttribute{
			Name:  "VisibilityTimeout",
			Value: strconv.Itoa(queue.VisibilityTimeoutSeconds),
		})
	}

	if shouldInclude("MaximumMessageSize") {
		attributes = append(attributes, QueueAttribute{
			Name:  "MaximumMessageSize",
			Value: "262144", // AWS default: 256 KB
		})
	}

	if shouldInclude("MessageRetentionPeriod") {
		attributes = append(attributes, QueueAttribute{
			Name:  "MessageRetentionPeriod",
			Value: strconv.Itoa(queue.MessageRetentionPeriod),
		})
	}

	if shouldInclude("DelaySeconds") {
		attributes = append(attributes, QueueAttribute{
			Name:  "DelaySeconds",
			Value: strconv.Itoa(queue.DelaySeconds),
		})
	}

	if shouldInclude("ReceiveMessageWaitTimeSeconds") {
		attributes = append(attributes, QueueAttribute{
			Name:  "ReceiveMessageWaitTimeSeconds",
			Value: strconv.Itoa(queue.ReceiveMessageWaitTimeSeconds),
		})
	}

	if shouldInclude("ApproximateNumberOfMessages") {
		attributes = append(attributes, QueueAttribute{
			Name:  "ApproximateNumberOfMessages",
			Value: strconv.Itoa(availableCount),
		})
	}

	if shouldInclude("ApproximateNumberOfMessagesNotVisible") {
		attributes = append(attributes, QueueAttribute{
			Name:  "ApproximateNumberOfMessagesNotVisible",
			Value: strconv.Itoa(inFlightCount),
		})
	}

	if shouldInclude("ApproximateNumberOfMessagesDelayed") {
		attributes = append(attributes, QueueAttribute{
			Name:  "ApproximateNumberOfMessagesDelayed",
			Value: strconv.Itoa(delayedCount),
		})
	}

	// Metadata attributes
	if shouldInclude("CreatedTimestamp") {
		attributes = append(attributes, QueueAttribute{
			Name:  "CreatedTimestamp",
			Value: strconv.FormatInt(queue.CreatedAt.Unix(), 10),
		})
	}

	if shouldInclude("LastModifiedTimestamp") {
		attributes = append(attributes, QueueAttribute{
			Name:  "LastModifiedTimestamp",
			Value: strconv.FormatInt(queue.CreatedAt.Unix(), 10), // For now, same as created
		})
	}

	if shouldInclude("QueueArn") {
		// Generate ARN format: arn:aws:sqs:region:account:queuename
		// For local testing, use fake values
		arn := fmt.Sprintf("arn:aws:sqs:us-east-1:123456789012:%s", queue.Name)
		attributes = append(attributes, QueueAttribute{
			Name:  "QueueArn",
			Value: arn,
		})
	}

	// DLQ attributes
	if shouldInclude("RedrivePolicy") && queue.RedrivePolicy != "" {
		attributes = append(attributes, QueueAttribute{
			Name:  "RedrivePolicy",
			Value: queue.RedrivePolicy,
		})
	}

	// Encryption attributes (basic support)
	if shouldInclude("SqsManagedSseEnabled") {
		attributes = append(attributes, QueueAttribute{
			Name:  "SqsManagedSseEnabled",
			Value: "false", // Default to false for now
		})
	}

	// FIFO-specific attributes
	if shouldInclude("FifoQueue") {
		attributes = append(attributes, QueueAttribute{
			Name:  "FifoQueue",
			Value: strconv.FormatBool(queue.FifoQueue),
		})
	}

	if shouldInclude("ContentBasedDeduplication") && queue.FifoQueue {
		attributes = append(attributes, QueueAttribute{
			Name:  "ContentBasedDeduplication",
			Value: strconv.FormatBool(queue.ContentBasedDeduplication),
		})
	}

	if shouldInclude("DeduplicationScope") && queue.FifoQueue {
		attributes = append(attributes, QueueAttribute{
			Name:  "DeduplicationScope",
			Value: queue.DeduplicationScope,
		})
	}

	if shouldInclude("FifoThroughputLimit") && queue.FifoQueue {
		attributes = append(attributes, QueueAttribute{
			Name:  "FifoThroughputLimit",
			Value: queue.FifoThroughputLimit,
		})
	}

	return attributes, nil
}

func (h *SMQHandler) handleSetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	// Get existing queue
	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	// Parse attributes from form
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

	// Update queue attributes
	if val, ok := attributes["VisibilityTimeout"]; ok {
		if timeout, err := strconv.Atoi(val); err == nil {
			queue.VisibilityTimeoutSeconds = timeout
		}
	}
	if val, ok := attributes["MessageRetentionPeriod"]; ok {
		if period, err := strconv.Atoi(val); err == nil {
			queue.MessageRetentionPeriod = period
		}
	}
	if val, ok := attributes["DelaySeconds"]; ok {
		if delay, err := strconv.Atoi(val); err == nil {
			queue.DelaySeconds = delay
		}
	}

	// Update queue in storage
	if err := h.storage.UpdateQueueAttributes(r.Context(), queueName, attributes); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to update queue attributes", http.StatusInternalServerError)
		return
	}

	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SMQHandler) handlePurgeQueue(w http.ResponseWriter, r *http.Request) {
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

func (h *SMQHandler) handleSendMessageBatch(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	var messages []*storage.Message
	var entries []BatchEntry

	entryIndex := 1
	for {
		idKey := fmt.Sprintf("Entry.%d.Id", entryIndex)
		bodyKey := fmt.Sprintf("Entry.%d.MessageBody", entryIndex)

		id := r.FormValue(idKey)
		body := r.FormValue(bodyKey)

		if id == "" || body == "" {
			break
		}

		// Create message
		message := &storage.Message{
			ID:                uuid.New().String(),
			QueueName:         queueName,
			Body:              body,
			Attributes:        make(map[string]string),
			MessageAttributes: make(map[string]storage.MessageAttribute),
			ReceiptHandle:     uuid.New().String(),
			ReceiveCount:      0,
			CreatedAt:         time.Now(),
			DelaySeconds:      0,
		}

		// Calculate MD5
		hash := md5.Sum([]byte(body))
		message.MD5OfBody = hex.EncodeToString(hash[:])

		// Calculate MD5 of message attributes
		message.MD5OfAttributes = calculateMD5OfMessageAttributes(message.MessageAttributes)

		// Parse DelaySeconds if provided
		delayKey := fmt.Sprintf("Entry.%d.DelaySeconds", entryIndex)
		if delayStr := r.FormValue(delayKey); delayStr != "" {
			if delay, err := strconv.Atoi(delayStr); err == nil {
				message.DelaySeconds = delay
			}
		}

		messages = append(messages, message)
		entries = append(entries, BatchEntry{
			Id:        id,
			MessageId: message.ID,
			MD5OfBody: message.MD5OfBody,
		})

		entryIndex++

		// AWS limit is 10 messages per batch
		if entryIndex > 10 {
			break
		}
	}

	if len(messages) == 0 {
		h.writeErrorResponse(w, "MissingParameter", "No valid batch entries found", http.StatusBadRequest)
		return
	}

	// Send messages using batch operation
	if err := h.storage.SendMessageBatch(r.Context(), messages); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to send message batch", http.StatusInternalServerError)
		return
	}

	response := SendMessageBatchResponse{
		Successful: entries,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SMQHandler) handleDeleteMessageBatch(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	// Parse batch entries
	var receiptHandles []string
	var entries []DeleteBatchEntry

	// Parse entries from form data (Entry.1.Id, Entry.1.ReceiptHandle, etc.)
	entryIndex := 1
	for {
		idKey := fmt.Sprintf("Entry.%d.Id", entryIndex)
		handleKey := fmt.Sprintf("Entry.%d.ReceiptHandle", entryIndex)

		id := r.FormValue(idKey)
		handle := r.FormValue(handleKey)

		if id == "" || handle == "" {
			break
		}

		receiptHandles = append(receiptHandles, handle)
		entries = append(entries, DeleteBatchEntry{
			Id: id,
		})

		entryIndex++

		// AWS limit is 10 messages per batch
		if entryIndex > 10 {
			break
		}
	}

	if len(receiptHandles) == 0 {
		h.writeErrorResponse(w, "MissingParameter", "No valid batch entries found", http.StatusBadRequest)
		return
	}

	// Delete messages using batch operation
	if err := h.storage.DeleteMessageBatch(r.Context(), queueName, receiptHandles); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to delete message batch", http.StatusInternalServerError)
		return
	}

	response := DeleteMessageBatchResponse{
		Successful: entries,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SMQHandler) handleChangeMessageVisibilityBatch(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	// Parse batch entries
	var visibilityEntries []storage.VisibilityEntry
	var responseEntries []VisibilityBatchEntry

	// Parse entries from form data
	entryIndex := 1
	for {
		idKey := fmt.Sprintf("Entry.%d.Id", entryIndex)
		handleKey := fmt.Sprintf("Entry.%d.ReceiptHandle", entryIndex)
		visibilityKey := fmt.Sprintf("Entry.%d.VisibilityTimeout", entryIndex)

		id := r.FormValue(idKey)
		handle := r.FormValue(handleKey)
		visibilityStr := r.FormValue(visibilityKey)

		if id == "" || handle == "" || visibilityStr == "" {
			break
		}

		visibilityTimeout, err := strconv.Atoi(visibilityStr)
		if err != nil {
			// Invalid visibility timeout - could add to failed entries
			entryIndex++
			continue
		}

		visibilityEntries = append(visibilityEntries, storage.VisibilityEntry{
			ReceiptHandle:     handle,
			VisibilityTimeout: visibilityTimeout,
		})

		responseEntries = append(responseEntries, VisibilityBatchEntry{
			Id: id,
		})

		entryIndex++

		// AWS limit is 10 messages per batch
		if entryIndex > 10 {
			break
		}
	}

	if len(visibilityEntries) == 0 {
		h.writeErrorResponse(w, "MissingParameter", "No valid batch entries found", http.StatusBadRequest)
		return
	}

	// Update visibility using batch operation
	if err := h.storage.ChangeMessageVisibilityBatch(r.Context(), queueName, visibilityEntries); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to change message visibility batch", http.StatusInternalServerError)
		return
	}

	response := ChangeMessageVisibilityBatchResponse{
		Successful: responseEntries,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SMQHandler) extractQueueNameFromURL(queueURL string) string {
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

func (h *SMQHandler) writeErrorResponse(w http.ResponseWriter, code, message string, statusCode int) {
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

func (h *SMQHandler) writeXMLResponse(w http.ResponseWriter, response interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)

	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
	xml.NewEncoder(w).Encode(response)
}

// JSON Protocol Response Functions
func (h *SMQHandler) writeJSONErrorResponse(w http.ResponseWriter, code, message string, statusCode int) {
	errorResp := JSONErrorResponse{
		Type:    fmt.Sprintf("com.amazon.sqs.api#%s", code),
		Code:    code,
		Message: message,
	}

	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errorResp)
}

func (h *SMQHandler) writeJSONResponse(w http.ResponseWriter, response interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// JSON SendMessage Handler
func (h *SMQHandler) handleJSONSendMessage(w http.ResponseWriter, r *http.Request) {
	var req JSONSendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	if req.MessageBody == "" {
		h.writeJSONErrorResponse(w, "MissingParameter", "MessageBody is required", http.StatusBadRequest)
		return
	}

	// Extract queue name from URL
	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	if queueName == "" {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Invalid queue URL", http.StatusBadRequest)
		return
	}

	// Check if queue exists
	ctx := context.Background()
	queue, err := h.storage.GetQueue(ctx, queueName)
	if err != nil || queue == nil {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	// Convert JSON message attributes to storage format
	storageAttrs := make(map[string]storage.MessageAttribute)
	for k, v := range req.MessageAttributes {
		storageAttrs[k] = storage.MessageAttribute{
			DataType:    v.DataType,
			StringValue: v.StringValue,
			BinaryValue: v.BinaryValue,
		}
	}

	// Create message
	messageID := uuid.New().String()
	md5Hash := md5.Sum([]byte(req.MessageBody))
	md5Hex := hex.EncodeToString(md5Hash[:])

	delaySeconds := 0
	if req.DelaySeconds != nil {
		delaySeconds = *req.DelaySeconds
	}

	message := &storage.Message{
		ID:                     messageID,
		QueueName:              queueName,
		Body:                   req.MessageBody,
		MessageAttributes:      storageAttrs,
		MessageDeduplicationId: req.MessageDeduplicationId,
		MessageGroupId:         req.MessageGroupId,
		DelaySeconds:           delaySeconds,
		CreatedAt:              time.Now(),
		MD5OfBody:              md5Hex,
	}

	// Calculate MD5 of message attributes
	message.MD5OfAttributes = calculateMD5OfMessageAttributes(storageAttrs)

	// Debug logging for checksums
	fmt.Printf("DEBUG: SendMessage JSON - MD5OfBody: %s, MD5OfAttributes: %s\n",
		message.MD5OfBody, message.MD5OfAttributes)

	// Handle FIFO queue logic
	if strings.HasSuffix(queueName, ".fifo") {
		// Get queue details for validation
		queue, err := h.storage.GetQueue(ctx, queueName)
		if err != nil {
			h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
			return
		}

		// Use existing FIFO validation
		if err := ValidateFifoMessage(message, queue); err != nil {
			h.writeJSONErrorResponse(w, "InvalidParameterValue", err.Error(), http.StatusBadRequest)
			return
		}

		// Prepare FIFO message (sequence numbers, deduplication, etc.)
		if err := PrepareFifoMessage(message, queue); err != nil {
			h.writeJSONErrorResponse(w, "InternalError", err.Error(), http.StatusInternalServerError)
			return
		}

		// Check for duplicate messages
		if message.DeduplicationHash != "" {
			isDuplicate, err := CheckForDuplicateMessage(ctx, h.storage, queueName, message.DeduplicationHash)
			if err != nil {
				h.writeJSONErrorResponse(w, "InternalError", "Failed to check for duplicates", http.StatusInternalServerError)
				return
			}

			if isDuplicate {
				// For duplicates, return success but don't actually send the message
				resp := JSONSendMessageResponse{
					MessageId:              messageID,
					MD5OfMessageBody:       md5Hex,
					MD5OfMessageAttributes: message.MD5OfAttributes,
					MessageDeduplicationId: message.MessageDeduplicationId,
					MessageGroupId:         message.MessageGroupId,
					SequenceNumber:         message.SequenceNumber,
				}
				h.writeJSONResponse(w, resp)
				return
			}
		}
	}

	// Send the message
	err = h.storage.SendMessage(ctx, message)
	if err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to send message", http.StatusInternalServerError)
		return
	}

	// Return response
	resp := JSONSendMessageResponse{
		MessageId:              messageID,
		MD5OfMessageBody:       md5Hex,
		MD5OfMessageAttributes: message.MD5OfAttributes,
		MessageDeduplicationId: message.MessageDeduplicationId,
		MessageGroupId:         message.MessageGroupId,
		SequenceNumber:         message.SequenceNumber,
	}

	h.writeJSONResponse(w, resp)
}

// JSON ReceiveMessage Handler
func (h *SMQHandler) handleJSONReceiveMessage(w http.ResponseWriter, r *http.Request) {
	var req JSONReceiveMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	if queueName == "" {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Invalid queue URL", http.StatusBadRequest)
		return
	}

	maxMessages := 1
	if req.MaxNumberOfMessages != nil {
		maxMessages = *req.MaxNumberOfMessages
	}

	visibilityTimeout := 30
	if req.VisibilityTimeout != nil {
		visibilityTimeout = *req.VisibilityTimeout
	}

	ctx := context.Background()
	messages, err := h.storage.ReceiveMessages(ctx, queueName, maxMessages, 0, visibilityTimeout)
	if err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to receive messages", http.StatusInternalServerError)
		return
	}

	var jsonMessages []JSONMessage
	for _, msg := range messages {
		// Convert storage message attributes to JSON format, but only include requested ones
		jsonAttrs := make(map[string]JSONMessageAttribute)

		// Only include MessageAttributes if they were requested
		if len(req.MessageAttributeNames) > 0 {
			includeAll := false
			requestedAttrs := make(map[string]bool)

			for _, attrName := range req.MessageAttributeNames {
				if attrName == "All" {
					includeAll = true
					break
				}
				requestedAttrs[attrName] = true
			}

			// Add requested message attributes
			for k, v := range msg.MessageAttributes {
				if includeAll || requestedAttrs[k] {
					jsonAttrs[k] = JSONMessageAttribute{
						DataType:    v.DataType,
						StringValue: v.StringValue,
						BinaryValue: v.BinaryValue,
					}
				}
			}
		}

		jsonMsg := JSONMessage{
			MessageId:              msg.ID,
			ReceiptHandle:          msg.ReceiptHandle,
			MD5OfBody:              msg.MD5OfBody, // Legacy field for backward compatibility
			MD5OfMessageBody:       msg.MD5OfBody, // Modern field (same value for compatibility)
			MD5OfMessageAttributes: msg.MD5OfAttributes,
			Body:                   msg.Body,
			MessageAttributes:      jsonAttrs,
			MessageGroupId:         msg.MessageGroupId,
			MessageDeduplicationId: msg.MessageDeduplicationId,
			SequenceNumber:         msg.SequenceNumber,
		}
		jsonMessages = append(jsonMessages, jsonMsg)
	}

	resp := JSONReceiveMessageResponse{
		Messages: jsonMessages,
	}

	h.writeJSONResponse(w, resp)
}

// JSON DeleteMessage Handler
func (h *SMQHandler) handleJSONDeleteMessage(w http.ResponseWriter, r *http.Request) {
	var req JSONDeleteMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	if req.ReceiptHandle == "" {
		h.writeJSONErrorResponse(w, "MissingParameter", "ReceiptHandle is required", http.StatusBadRequest)
		return
	}

	// Extract queue name from URL
	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	if queueName == "" {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Invalid queue URL", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	err := h.storage.DeleteMessage(ctx, queueName, req.ReceiptHandle)
	if err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to delete message", http.StatusInternalServerError)
		return
	}

	// Return empty JSON object for successful delete
	h.writeJSONResponse(w, map[string]interface{}{})
}

// JSON DeleteMessageBatch Handler
func (h *SMQHandler) handleJSONDeleteMessageBatch(w http.ResponseWriter, r *http.Request) {
	var req JSONDeleteMessageBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	if len(req.Entries) == 0 {
		h.writeJSONErrorResponse(w, "MissingParameter", "No valid batch entries found", http.StatusBadRequest)
		return
	}

	// Extract queue name from URL
	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	if queueName == "" {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Invalid queue URL", http.StatusBadRequest)
		return
	}

	// Process each entry individually to handle partial failures
	var successful []JSONBatchResultEntry
	var failed []JSONBatchResultErrorEntry
	ctx := context.Background()

	for _, entry := range req.Entries {
		if entry.Id == "" || entry.ReceiptHandle == "" {
			failed = append(failed, JSONBatchResultErrorEntry{
				Id:          entry.Id,
				Code:        "MissingParameter",
				Message:     "Id and ReceiptHandle are required",
				SenderFault: true,
			})
			continue
		}

		// Try to delete this specific message
		if err := h.storage.DeleteMessage(ctx, queueName, entry.ReceiptHandle); err != nil {
			failed = append(failed, JSONBatchResultErrorEntry{
				Id:          entry.Id,
				Code:        "ReceiptHandleIsInvalid",
				Message:     "The specified receipt handle is not valid",
				SenderFault: true,
			})
		} else {
			successful = append(successful, JSONBatchResultEntry{
				Id: entry.Id,
			})
		}
	}

	response := JSONDeleteMessageBatchResponse{
		Successful: successful,
		Failed:     failed,
	}

	h.writeJSONResponse(w, response)
}

// JSON SendMessageBatch Handler
func (h *SMQHandler) handleJSONSendMessageBatch(w http.ResponseWriter, r *http.Request) {
	var req JSONSendMessageBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	if req.QueueUrl == "" {
		h.writeJSONErrorResponse(w, "MissingParameter", "QueueUrl is required", http.StatusBadRequest)
		return
	}

	if len(req.Entries) == 0 {
		h.writeJSONErrorResponse(w, "EmptyBatchRequest", "No entries in batch request", http.StatusBadRequest)
		return
	}

	if len(req.Entries) > 10 {
		h.writeJSONErrorResponse(w, "TooManyEntriesInBatchRequest", "Maximum of 10 entries allowed", http.StatusBadRequest)
		return
	}

	// Extract queue name from URL
	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	if queueName == "" {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Invalid queue URL", http.StatusBadRequest)
		return
	}

	// Check if queue exists
	ctx := context.Background()
	queue, err := h.storage.GetQueue(ctx, queueName)
	if err != nil || queue == nil {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	var messages []*storage.Message
	var successful []JSONBatchResultEntry
	var failed []JSONBatchResultErrorEntry

	for _, entry := range req.Entries {
		// Validate entry
		if entry.Id == "" {
			failed = append(failed, JSONBatchResultErrorEntry{
				Id:          entry.Id,
				SenderFault: true,
				Code:        "MissingParameter",
				Message:     "Entry Id is required",
			})
			continue
		}

		if entry.MessageBody == "" {
			failed = append(failed, JSONBatchResultErrorEntry{
				Id:          entry.Id,
				SenderFault: true,
				Code:        "MissingParameter",
				Message:     "MessageBody is required",
			})
			continue
		}

		// Create message
		messageID := uuid.New().String()
		message := &storage.Message{
			ID:                     messageID,
			QueueName:              queueName,
			Body:                   entry.MessageBody,
			Attributes:             make(map[string]string),
			MessageAttributes:      make(map[string]storage.MessageAttribute),
			ReceiptHandle:          uuid.New().String(),
			ReceiveCount:           0,
			CreatedAt:              time.Now(),
			DelaySeconds:           0,
			MessageDeduplicationId: entry.MessageDeduplicationId,
			MessageGroupId:         entry.MessageGroupId,
		}

		// Set DelaySeconds if provided
		if entry.DelaySeconds != nil {
			message.DelaySeconds = *entry.DelaySeconds
		}

		// Convert JSON message attributes to storage format
		for name, attr := range entry.MessageAttributes {
			message.MessageAttributes[name] = storage.MessageAttribute{
				DataType:    attr.DataType,
				StringValue: attr.StringValue,
				BinaryValue: attr.BinaryValue,
			}
		}

		// Calculate MD5 checksums
		hash := md5.Sum([]byte(entry.MessageBody))
		message.MD5OfBody = hex.EncodeToString(hash[:])

		// Calculate MD5 of message attributes
		message.MD5OfAttributes = calculateMD5OfMessageAttributes(message.MessageAttributes)

		messages = append(messages, message)

		// Add to successful entries
		successful = append(successful, JSONBatchResultEntry{
			Id:                     entry.Id,
			MessageId:              messageID,
			MD5OfMessageBody:       message.MD5OfBody,
			MD5OfMessageAttributes: message.MD5OfAttributes,
			MessageDeduplicationId: message.MessageDeduplicationId,
			MessageGroupId:         message.MessageGroupId,
			SequenceNumber:         message.SequenceNumber,
		})
	}

	// Send messages if any are valid
	if len(messages) > 0 {
		if err := h.storage.SendMessageBatch(ctx, messages); err != nil {
			h.writeJSONErrorResponse(w, "InternalError", "Failed to send message batch", http.StatusInternalServerError)
			return
		}
	}

	// Return response
	resp := JSONSendMessageBatchResponse{
		Successful: successful,
		Failed:     failed,
	}

	h.writeJSONResponse(w, resp)
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

func convertAttributes(attrs map[string]string) []Attribute {
	var result []Attribute
	for k, v := range attrs {
		result = append(result, Attribute{
			Name:  k,
			Value: v,
		})
	}
	return result
}

func convertMessageAttributesToSlice(attrs map[string]storage.MessageAttribute) []MessageAttributeValue {
	var result []MessageAttributeValue
	for k, v := range attrs {
		result = append(result, MessageAttributeValue{
			Name:        k,
			StringValue: v.StringValue,
			DataType:    v.DataType,
		})
	}
	return result
}

// Authentication handlers
func (h *SMQHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Check if admin credentials are configured
		if h.adminUsername == "" || h.adminPassword == "" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
    <title>Simple Message Queue - Setup Required</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 500px; margin: 0 auto; background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .error { color: #e74c3c; }
        .code { background: #f8f9fa; padding: 15px; border-radius: 5px; font-family: monospace; margin: 15px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Setup Required</h1>
        <p class="error">Admin credentials are not configured.</p>
        <p>To access the Simple Message Queue dashboard, please set the following environment variables:</p>
        <div class="code">
ADMIN_USERNAME=your_admin_username<br>
ADMIN_PASSWORD=your_secure_password
        </div>
        <p>Then restart the Simple Message Queue service.</p>
    </div>
</body>
</html>`))
			return
		}

		// Show login page
		w.Header().Set("Content-Type", "text/html")
		html, err := os.ReadFile("src/templates/login.html")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Error loading login template"))
			return
		}
		w.Write(html)
		return
	}

	if r.Method == http.MethodPost {
		// Process login
		username := r.FormValue("username")
		password := r.FormValue("password")

		// Check credentials
		if h.adminUsername == "" || h.adminPassword == "" {
			http.Error(w, "Admin credentials not configured", http.StatusServiceUnavailable)
			return
		}

		if username == h.adminUsername && password == h.adminPassword {
			// Create session cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "sqs_session",
				Value:    "authenticated",
				Path:     "/",
				HttpOnly: true,
				Secure:   false, // Set to true if using HTTPS
				SameSite: http.SameSiteLaxMode,
				MaxAge:   24 * 60 * 60, // 24 hours
			})
			w.WriteHeader(http.StatusOK)
			return
		}

		// Invalid credentials
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (h *SMQHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "sqs_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1, // Delete cookie
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *SMQHandler) isAuthenticated(r *http.Request) bool {
	// If no admin credentials are configured, deny access
	if h.adminUsername == "" || h.adminPassword == "" {
		return false
	}

	cookie, err := r.Cookie("sqs_session")
	if err != nil {
		return false
	}
	return cookie.Value == "authenticated"
}

func (h *SMQHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.isAuthenticated(r) {
			// Include current URL as redirect parameter
			redirectURL := "/login?redirect=" + url.QueryEscape(r.URL.String())
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (h *SMQHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	// Read the dashboard HTML template
	html, err := os.ReadFile("src/templates/dashboard.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error loading dashboard template"))
		return
	}

	w.Write(html)
}

func (h *SMQHandler) handleQueueDetails(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	// Read the queue details HTML template
	html, err := os.ReadFile("src/templates/queue-details.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error loading queue details template"))
		return
	}

	w.Write(html)
}

func (h *SMQHandler) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
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

// handleAPIRoutes handles REST API endpoints for dashboard operations
// API handlers for dashboard operations
func (h *SMQHandler) handleAPIListQueues(w http.ResponseWriter, r *http.Request) {
	queues, err := h.storage.ListQueues(r.Context(), "")
	if err != nil {
		http.Error(w, `{"error": "Failed to list queues"}`, http.StatusInternalServerError)
		return
	}

	response := `{"queues": [`
	for i, queue := range queues {
		if i > 0 {
			response += ","
		}
		response += fmt.Sprintf(`{
			"name": "%s",
			"url": "%s/%s",
			"createdAt": "%s"
		}`, queue.Name, h.baseURL, queue.Name, queue.CreatedAt.Format(time.RFC3339))
	}
	response += `]}`

	w.Write([]byte(response))
}

func (h *SMQHandler) handleAPICreateQueue(w http.ResponseWriter, r *http.Request) {
	var request struct {
		QueueName string `json:"queueName"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	queue := &storage.Queue{
		Name:      request.QueueName,
		CreatedAt: time.Now(),
	}

	if err := h.storage.CreateQueue(r.Context(), queue); err != nil {
		http.Error(w, `{"error": "Failed to create queue"}`, http.StatusInternalServerError)
		return
	}

	response := fmt.Sprintf(`{
		"success": true,
		"queueUrl": "%s/%s"
	}`, h.baseURL, request.QueueName)

	w.Write([]byte(response))
}

func (h *SMQHandler) handleAPIPollMessages(w http.ResponseWriter, r *http.Request) {
	var request struct {
		QueueName   string `json:"queueName"`
		MaxMessages int    `json:"maxMessages"`
		WaitTime    int    `json:"waitTimeSeconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if request.MaxMessages == 0 {
		request.MaxMessages = 10
	}

	messages, err := h.storage.ReceiveMessages(r.Context(), request.QueueName, request.MaxMessages, request.WaitTime, 0) // Use queue default visibility timeout
	if err != nil {
		http.Error(w, `{"error": "Failed to poll messages"}`, http.StatusInternalServerError)
		return
	}

	response := `{"messages": [`
	for i, msg := range messages {
		if i > 0 {
			response += ","
		}
		response += fmt.Sprintf(`{
			"id": "%s",
			"receiptHandle": "%s",
			"body": %s,
			"attributes": {
				"SentTimestamp": "%d",
				"ApproximateReceiveCount": "%d"
			},
			"md5OfBody": "%s"
		}`, msg.ID, msg.ReceiptHandle, strconv.Quote(msg.Body),
			msg.CreatedAt.UnixMilli(), msg.ReceiveCount, msg.MD5OfBody)
	}
	response += `]}`

	w.Write([]byte(response))
}

func (h *SMQHandler) handleAPISendMessage(w http.ResponseWriter, r *http.Request) {
	var request struct {
		QueueName string `json:"queueName"`
		Body      string `json:"messageBody"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	message := &storage.Message{
		ID:        uuid.New().String(),
		QueueName: request.QueueName,
		Body:      request.Body,
		CreatedAt: time.Now(),
	}

	hasher := md5.New()
	hasher.Write([]byte(request.Body))
	message.MD5OfBody = hex.EncodeToString(hasher.Sum(nil))

	if err := h.storage.SendMessage(r.Context(), message); err != nil {
		http.Error(w, `{"error": "Failed to send message"}`, http.StatusInternalServerError)
		return
	}

	response := fmt.Sprintf(`{
		"success": true,
		"messageId": "%s",
		"md5OfBody": "%s"
	}`, message.ID, message.MD5OfBody)

	w.Write([]byte(response))
}

func (h *SMQHandler) handleAPIDeleteMessage(w http.ResponseWriter, r *http.Request) {
	var request struct {
		QueueName     string `json:"queueName"`
		ReceiptHandle string `json:"receiptHandle"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if err := h.storage.DeleteMessage(r.Context(), request.QueueName, request.ReceiptHandle); err != nil {
		http.Error(w, `{"error": "Failed to delete message"}`, http.StatusInternalServerError)
		return
	}

	w.Write([]byte(`{"success": true}`))
}

func (h *SMQHandler) handleAPIDeleteQueue(w http.ResponseWriter, r *http.Request) {
	// Extract queue name from path like /api/queues/my-queue
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 4 {
		http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
		return
	}
	queueName := pathParts[3]

	if err := h.storage.DeleteQueue(r.Context(), queueName); err != nil {
		http.Error(w, `{"error": "Failed to delete queue"}`, http.StatusInternalServerError)
		return
	}

	w.Write([]byte(`{"success": true}`))
}

func (h *SMQHandler) handleAPIGetQueueMessages(w http.ResponseWriter, r *http.Request) {
	// Extract queue name from path like /api/queues/my-queue/messages
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 5 {
		http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
		return
	}
	queueName := pathParts[3]

	messages, err := h.storage.GetInFlightMessages(r.Context(), queueName)
	if err != nil {
		http.Error(w, `{"error": "Failed to get messages"}`, http.StatusInternalServerError)
		return
	}

	response := `{"messages": [`
	for i, msg := range messages {
		if i > 0 {
			response += ","
		}
		response += fmt.Sprintf(`{
			"id": "%s",
			"body": %s,
			"sentTimestamp": "%s",
			"receiveCount": %d,
			"attributes": {
				"SentTimestamp": "%d",
				"ApproximateReceiveCount": "%d"
			}
		}`, msg.ID, strconv.Quote(msg.Body),
			msg.CreatedAt.Format(time.RFC3339), msg.ReceiveCount,
			msg.CreatedAt.UnixMilli(), msg.ReceiveCount)
	}
	response += `]}`

	w.Write([]byte(response))
}

func (h *SMQHandler) handleAPIGetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	// Extract queue name from path like /api/queues/my-queue/attributes
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 5 {
		http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
		return
	}
	queueName := pathParts[3]

	// Get queue details
	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		http.Error(w, `{"error": "Queue not found"}`, http.StatusNotFound)
		return
	}

	// Get live message counts
	allMessages, err := h.storage.GetInFlightMessages(r.Context(), queueName)
	if err != nil {
		http.Error(w, `{"error": "Failed to get messages"}`, http.StatusInternalServerError)
		return
	}

	now := time.Now()
	var availableCount, inFlightCount int

	for _, msg := range allMessages {
		// Check if message is in flight (invisible)
		if msg.VisibleAt != nil && now.Before(*msg.VisibleAt) {
			inFlightCount++
		} else {
			availableCount++
		}
	}

	response := fmt.Sprintf(`{
		"availableMessages": %d,
		"inFlightMessages": %d,
		"totalMessages": %d
	}`, availableCount, inFlightCount, availableCount+inFlightCount)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(response))
}

func (h *SMQHandler) handleAPIPurgeQueue(w http.ResponseWriter, r *http.Request) {
	// Extract queue name from path like /api/queues/my-queue/purge
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 5 {
		http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
		return
	}
	queueName := pathParts[3]

	if err := h.storage.PurgeQueue(r.Context(), queueName); err != nil {
		http.Error(w, `{"error": "Failed to purge queue"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true}`))
}

func (h *SMQHandler) handleAPIRedriveMessage(w http.ResponseWriter, r *http.Request) {
	// Extract queue name from path like /api/queues/my-queue/redrive
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 5 {
		http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
		return
	}
	dlqName := pathParts[3]

	var request struct {
		MessageId string `json:"messageId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Get the source queue for this DLQ
	sourceQueueName, err := h.storage.GetSourceQueueForDLQ(r.Context(), dlqName)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to find source queue: %s"}`, err.Error()), http.StatusNotFound)
		return
	}

	// Redrive the message
	err = h.storage.RedriveMessage(r.Context(), dlqName, request.MessageId, sourceQueueName)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to redrive message: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true, "message": "Message redriven successfully"}`))
}

func (h *SMQHandler) handleAPIRedriveBatch(w http.ResponseWriter, r *http.Request) {
	// Extract queue name from path like /api/queues/my-queue/redrive-batch
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 5 {
		http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
		return
	}
	dlqName := pathParts[3]

	var request struct {
		MessageIds []string `json:"messageIds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Get the source queue for this DLQ
	sourceQueueName, err := h.storage.GetSourceQueueForDLQ(r.Context(), dlqName)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to find source queue: %s"}`, err.Error()), http.StatusNotFound)
		return
	}

	// Redrive the messages
	err = h.storage.RedriveMessageBatch(r.Context(), dlqName, request.MessageIds, sourceQueueName)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to redrive messages: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"success": true, "message": "Successfully redriven %d messages"}`, len(request.MessageIds))))
}

// handleHealthCheck returns basic health status for Docker health checks
func (h *SMQHandler) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// Check database connectivity
	ctx := r.Context()
	_, err := h.storage.ListQueues(ctx, "")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "unhealthy", "error": "database connection failed"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "healthy", "service": "simple-message-queue"}`))
}

// JSON CreateQueue Handler
func (h *SMQHandler) handleJSONCreateQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueName  string            `json:"QueueName"`
		Attributes map[string]string `json:"Attributes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	if req.QueueName == "" {
		h.writeJSONErrorResponse(w, "MissingParameter", "QueueName is required", http.StatusBadRequest)
		return
	}

	// Create queue object (similar to form handler)
	queue := &storage.Queue{
		Name:                          req.QueueName,
		URL:                           fmt.Sprintf("%s/%s", h.baseURL, req.QueueName),
		Attributes:                    req.Attributes,
		VisibilityTimeoutSeconds:      30,
		MessageRetentionPeriod:        1209600, // 14 days
		MaxReceiveCount:               0,
		DelaySeconds:                  0,
		ReceiveMessageWaitTimeSeconds: 0,
		CreatedAt:                     time.Now(),
	}

	// Parse standard attributes (similar to form handler)
	if req.Attributes != nil {
		if val, ok := req.Attributes["VisibilityTimeout"]; ok {
			if timeout, err := strconv.Atoi(val); err == nil {
				queue.VisibilityTimeoutSeconds = timeout
			}
		}
		if val, ok := req.Attributes["MessageRetentionPeriod"]; ok {
			if period, err := strconv.Atoi(val); err == nil {
				queue.MessageRetentionPeriod = period
			}
		}
		if val, ok := req.Attributes["FifoQueue"]; ok {
			queue.FifoQueue = (val == "true")
		}
		if val, ok := req.Attributes["ContentBasedDeduplication"]; ok {
			queue.ContentBasedDeduplication = (val == "true")
		}
	}

	// Setup FIFO queue configuration
	if err := SetupFifoQueue(queue); err != nil {
		h.writeJSONErrorResponse(w, "InvalidParameterValue", err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.storage.CreateQueue(r.Context(), queue); err != nil {
		h.writeJSONErrorResponse(w, "QueueAlreadyExists", "Queue already exists", http.StatusBadRequest)
		return
	}

	resp := map[string]string{
		"QueueUrl": queue.URL,
	}
	h.writeJSONResponse(w, resp)
}

// JSON DeleteQueue Handler
func (h *SMQHandler) handleJSONDeleteQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string `json:"QueueUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	if err := h.storage.DeleteQueue(r.Context(), queueName); err != nil {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	h.writeJSONResponse(w, map[string]interface{}{})
}

// JSON ListQueues Handler
func (h *SMQHandler) handleJSONListQueues(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueNamePrefix string `json:"QueueNamePrefix,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req) // Ignore error for optional body

	queues, err := h.storage.ListQueues(r.Context(), req.QueueNamePrefix)
	if err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to list queues", http.StatusInternalServerError)
		return
	}

	var queueURLs []string
	for _, queue := range queues {
		queueURLs = append(queueURLs, queue.URL)
	}

	resp := map[string][]string{
		"QueueUrls": queueURLs,
	}
	h.writeJSONResponse(w, resp)
}

// JSON GetQueueUrl Handler
func (h *SMQHandler) handleJSONGetQueueUrl(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueName string `json:"QueueName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	queue, err := h.storage.GetQueue(r.Context(), req.QueueName)
	if err != nil || queue == nil {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	resp := map[string]string{
		"QueueUrl": queue.URL,
	}
	h.writeJSONResponse(w, resp)
}

// JSON ChangeMessageVisibility Handler
func (h *SMQHandler) handleJSONChangeMessageVisibility(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl          string `json:"QueueUrl"`
		ReceiptHandle     string `json:"ReceiptHandle"`
		VisibilityTimeout int    `json:"VisibilityTimeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	if err := h.storage.ChangeMessageVisibility(r.Context(), queueName, req.ReceiptHandle, req.VisibilityTimeout); err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to change message visibility", http.StatusInternalServerError)
		return
	}

	h.writeJSONResponse(w, map[string]interface{}{})
}

// JSON ChangeMessageVisibilityBatch Handler
func (h *SMQHandler) handleJSONChangeMessageVisibilityBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string `json:"QueueUrl"`
		Entries  []struct {
			Id                string `json:"Id"`
			ReceiptHandle     string `json:"ReceiptHandle"`
			VisibilityTimeout int    `json:"VisibilityTimeout"`
		} `json:"Entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	queueName := h.extractQueueNameFromURL(req.QueueUrl)

	var visibilityEntries []storage.VisibilityEntry
	var successful []JSONBatchResultEntry

	for _, entry := range req.Entries {
		visibilityEntries = append(visibilityEntries, storage.VisibilityEntry{
			ReceiptHandle:     entry.ReceiptHandle,
			VisibilityTimeout: entry.VisibilityTimeout,
		})
		successful = append(successful, JSONBatchResultEntry{Id: entry.Id})
	}

	if err := h.storage.ChangeMessageVisibilityBatch(r.Context(), queueName, visibilityEntries); err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to change message visibility batch", http.StatusInternalServerError)
		return
	}

	resp := map[string][]JSONBatchResultEntry{
		"Successful": successful,
	}
	h.writeJSONResponse(w, resp)
}

// JSON GetQueueAttributes Handler
func (h *SMQHandler) handleJSONGetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl       string   `json:"QueueUrl"`
		AttributeNames []string `json:"AttributeNames,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	// Default to "All" if no attributes specified
	if len(req.AttributeNames) == 0 {
		req.AttributeNames = []string{"All"}
	}

	// Generate attributes (reuse existing logic)
	attributes, err := h.generateQueueAttributes(r.Context(), queue, req.AttributeNames)
	if err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to generate queue attributes", http.StatusInternalServerError)
		return
	}

	// Convert to map format for JSON
	attrMap := make(map[string]string)
	for _, attr := range attributes {
		attrMap[attr.Name] = attr.Value
	}

	resp := map[string]map[string]string{
		"Attributes": attrMap,
	}
	h.writeJSONResponse(w, resp)
}

// JSON SetQueueAttributes Handler
func (h *SMQHandler) handleJSONSetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl   string            `json:"QueueUrl"`
		Attributes map[string]string `json:"Attributes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	queueName := h.extractQueueNameFromURL(req.QueueUrl)

	// Check if queue exists
	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeJSONErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	// Update queue attributes using storage interface
	if err := h.storage.UpdateQueueAttributes(r.Context(), queueName, req.Attributes); err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to update queue attributes", http.StatusInternalServerError)
		return
	}

	h.writeJSONResponse(w, map[string]interface{}{})
}

// JSON PurgeQueue Handler
func (h *SMQHandler) handleJSONPurgeQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string `json:"QueueUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeJSONErrorResponse(w, "InvalidRequest", "Failed to parse JSON request", http.StatusBadRequest)
		return
	}

	queueName := h.extractQueueNameFromURL(req.QueueUrl)
	if err := h.storage.PurgeQueue(r.Context(), queueName); err != nil {
		h.writeJSONErrorResponse(w, "InternalError", "Failed to purge queue", http.StatusInternalServerError)
		return
	}

	h.writeJSONResponse(w, map[string]interface{}{})
}
