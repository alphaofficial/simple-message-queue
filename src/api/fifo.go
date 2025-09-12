package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sqs-backend/src/storage"
)

// FIFO utility functions

// IsFifoQueue checks if a queue name has the .fifo suffix
func IsFifoQueue(queueName string) bool {
	return strings.HasSuffix(queueName, ".fifo")
}

// ValidateFifoQueueName validates that a FIFO queue name is properly formatted
func ValidateFifoQueueName(queueName string) error {
	if !IsFifoQueue(queueName) {
		return fmt.Errorf("FIFO queue name must end with .fifo suffix")
	}

	// Queue name without .fifo must be 1-75 characters (AWS limit is 80 total with .fifo)
	baseName := strings.TrimSuffix(queueName, ".fifo")
	if len(baseName) == 0 || len(baseName) > 75 {
		return fmt.Errorf("FIFO queue base name must be 1-75 characters")
	}

	return nil
}

// ValidateFifoMessage validates FIFO-specific message requirements
func ValidateFifoMessage(message *storage.Message, queue *storage.Queue) error {
	if !queue.FifoQueue {
		// Standard queue - FIFO fields should be empty
		if message.MessageGroupId != "" {
			return fmt.Errorf("MessageGroupId is only valid for FIFO queues")
		}
		if message.MessageDeduplicationId != "" {
			return fmt.Errorf("MessageDeduplicationId is only valid for FIFO queues")
		}
		return nil
	}

	// FIFO queue validations
	if message.MessageGroupId == "" {
		return fmt.Errorf("MessageGroupId is required for FIFO queues")
	}

	// MessageGroupId must be 1-128 characters
	if len(message.MessageGroupId) > 128 {
		return fmt.Errorf("MessageGroupId must be 1-128 characters")
	}

	// MessageDeduplicationId validation (if provided)
	if message.MessageDeduplicationId != "" && len(message.MessageDeduplicationId) > 128 {
		return fmt.Errorf("MessageDeduplicationId must be up to 128 characters")
	}

	// If ContentBasedDeduplication is disabled, MessageDeduplicationId is required
	if !queue.ContentBasedDeduplication && message.MessageDeduplicationId == "" {
		return fmt.Errorf("MessageDeduplicationId is required when ContentBasedDeduplication is disabled")
	}

	return nil
}

// GenerateSequenceNumber generates a sequence number for FIFO ordering
func GenerateSequenceNumber() string {
	// Use high-precision timestamp + counter for ordering
	now := time.Now()
	nanos := now.UnixNano()
	return strconv.FormatInt(nanos, 10)
}

// GenerateContentBasedDeduplicationId generates a deduplication ID based on message content
func GenerateContentBasedDeduplicationId(body string, messageAttributes map[string]storage.MessageAttribute) string {
	h := sha256.New()
	h.Write([]byte(body))

	// Include message attributes in hash for complete content-based deduplication
	for key, attr := range messageAttributes {
		h.Write([]byte(key))
		h.Write([]byte(attr.DataType))
		h.Write([]byte(attr.StringValue))
		h.Write(attr.BinaryValue)
	}

	hash := h.Sum(nil)
	return hex.EncodeToString(hash)
}

// GenerateDeduplicationHash generates a SHA-256 hash for deduplication purposes
func GenerateDeduplicationHash(queueName, messageDeduplicationId string) string {
	h := sha256.New()
	h.Write([]byte(queueName))
	h.Write([]byte(messageDeduplicationId))
	hash := h.Sum(nil)
	return hex.EncodeToString(hash)
}

// SetupFifoQueue configures FIFO-specific queue attributes
func SetupFifoQueue(queue *storage.Queue) error {
	// Check if FifoQueue attribute is explicitly set to true
	fifoAttrSet := false
	for key, value := range queue.Attributes {
		if key == "FifoQueue" && value == "true" {
			fifoAttrSet = true
			break
		}
	}

	isFifoByName := IsFifoQueue(queue.Name)

	// Validate FIFO queue naming consistency
	if fifoAttrSet && !isFifoByName {
		return fmt.Errorf("FIFO queue name must end with .fifo suffix")
	}

	if !isFifoByName && !fifoAttrSet {
		return nil // Not a FIFO queue
	}

	// Set FIFO flag
	queue.FifoQueue = true

	// Set default FIFO attributes if not specified
	if queue.DeduplicationScope == "" {
		queue.DeduplicationScope = "queue" // Default: deduplication across entire queue
	}

	if queue.FifoThroughputLimit == "" {
		queue.FifoThroughputLimit = "perQueue" // Default: standard throughput
	}

	// FIFO queues don't support DelaySeconds > 0
	if queue.DelaySeconds > 0 {
		return fmt.Errorf("FIFO queues do not support DelaySeconds > 0")
	}

	return nil
}

// PrepareFifoMessage prepares a message for FIFO queue storage
func PrepareFifoMessage(message *storage.Message, queue *storage.Queue) error {
	if !queue.FifoQueue {
		return nil // Not a FIFO queue
	}

	// Generate sequence number for ordering
	message.SequenceNumber = GenerateSequenceNumber()

	// Handle deduplication
	if message.MessageDeduplicationId == "" && queue.ContentBasedDeduplication {
		// Generate content-based deduplication ID
		message.MessageDeduplicationId = GenerateContentBasedDeduplicationId(message.Body, message.MessageAttributes)
	}

	// Generate deduplication hash for storage indexing
	if message.MessageDeduplicationId != "" {
		message.DeduplicationHash = GenerateDeduplicationHash(queue.Name, message.MessageDeduplicationId)
	}

	return nil
}

// CheckForDuplicateMessage checks if a message with the same deduplication ID was sent recently
func CheckForDuplicateMessage(ctx context.Context, storage storage.Storage, queueName, deduplicationHash string) (bool, error) {
	// AWS SQS deduplication window is 5 minutes
	deduplicationWindow := 5 * time.Minute
	return storage.CheckForDuplicate(ctx, queueName, deduplicationHash, deduplicationWindow)
}
