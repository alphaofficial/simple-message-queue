package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"simple-message-queue/src/storage"
)

func IsFifoQueue(queueName string) bool {
	return strings.HasSuffix(queueName, ".fifo")
}

func ValidateFifoQueueName(queueName string) error {
	if !IsFifoQueue(queueName) {
		return fmt.Errorf("FIFO queue name must end with .fifo suffix")
	}

	baseName := strings.TrimSuffix(queueName, ".fifo")
	if len(baseName) == 0 || len(baseName) > 75 {
		return fmt.Errorf("FIFO queue base name must be 1-75 characters")
	}

	return nil
}

func ValidateFifoMessage(message *storage.Message, queue *storage.Queue) error {
	if !queue.FifoQueue {
		if message.MessageGroupId != "" {
			return fmt.Errorf("MessageGroupId is only valid for FIFO queues")
		}
		if message.MessageDeduplicationId != "" {
			return fmt.Errorf("MessageDeduplicationId is only valid for FIFO queues")
		}
		return nil
	}

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

func GenerateSequenceNumber() string {
	now := time.Now()
	nanos := now.UnixNano()
	return strconv.FormatInt(nanos, 10)
}

func GenerateContentBasedDeduplicationId(body string, messageAttributes map[string]storage.MessageAttribute) string {
	h := sha256.New()
	h.Write([]byte(body))

	for key, attr := range messageAttributes {
		h.Write([]byte(key))
		h.Write([]byte(attr.DataType))
		h.Write([]byte(attr.StringValue))
		h.Write(attr.BinaryValue)
	}

	hash := h.Sum(nil)
	return hex.EncodeToString(hash)
}

func GenerateDeduplicationHash(queueName, messageDeduplicationId string) string {
	h := sha256.New()
	h.Write([]byte(queueName))
	h.Write([]byte(messageDeduplicationId))
	hash := h.Sum(nil)
	return hex.EncodeToString(hash)
}

func SetupFifoQueue(queue *storage.Queue) error {
	fifoAttrSet := false
	for key, value := range queue.Attributes {
		if key == "FifoQueue" && value == "true" {
			fifoAttrSet = true
			break
		}
	}

	isFifoByName := IsFifoQueue(queue.Name)

	if fifoAttrSet && !isFifoByName {
		return fmt.Errorf("FIFO queue name must end with .fifo suffix")
	}

	if !isFifoByName && !fifoAttrSet {
		return nil
	}

	queue.FifoQueue = true

	if queue.DeduplicationScope == "" {
		queue.DeduplicationScope = "queue" // Default: deduplication across entire queue
	}

	if queue.FifoThroughputLimit == "" {
		queue.FifoThroughputLimit = "perQueue" // Default: standard throughput
	}

	if queue.DelaySeconds > 0 {
		return fmt.Errorf("FIFO queues do not support DelaySeconds > 0")
	}

	return nil
}

func PrepareFifoMessage(message *storage.Message, queue *storage.Queue) error {
	if !queue.FifoQueue {
		return nil // Not a FIFO queue
	}

	message.SequenceNumber = GenerateSequenceNumber()

	if message.MessageDeduplicationId == "" && queue.ContentBasedDeduplication {
		message.MessageDeduplicationId = GenerateContentBasedDeduplicationId(message.Body, message.MessageAttributes)
	}

	if message.MessageDeduplicationId != "" {
		message.DeduplicationHash = GenerateDeduplicationHash(queue.Name, message.MessageDeduplicationId)
	}

	return nil
}

func CheckForDuplicateMessage(ctx context.Context, storage storage.Storage, queueName, deduplicationHash string) (bool, error) {
	// AWS SQS deduplication window is 5 minutes
	deduplicationWindow := 5 * time.Minute
	return storage.CheckForDuplicate(ctx, queueName, deduplicationHash, deduplicationWindow)
}
