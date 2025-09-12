package sqlite

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	storage "sqs-backend/src/storage"
)

type SQLiteStorage struct {
	db *sql.DB
}

func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	// Use WAL mode for better concurrency
	db, err := sql.Open("sqlite3", dbPath+"?_journal=WAL&_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite works better with single connection
	db.SetMaxIdleConns(1)

	s := &SQLiteStorage{db: db}
	if err := s.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return s, nil
}

func (s *SQLiteStorage) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS queues (
			name TEXT PRIMARY KEY,
			url TEXT NOT NULL,
			attributes TEXT NOT NULL,
			visibility_timeout_seconds INTEGER DEFAULT 30,
			message_retention_period INTEGER DEFAULT 1209600,
			max_receive_count INTEGER DEFAULT 0,
			delay_seconds INTEGER DEFAULT 0,
			receive_message_wait_time_seconds INTEGER DEFAULT 0,
			dead_letter_queue_name TEXT,
			redrive_policy TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			-- FIFO queue fields
			fifo_queue BOOLEAN DEFAULT FALSE,
			content_based_deduplication BOOLEAN DEFAULT FALSE,
			deduplication_scope TEXT DEFAULT 'queue',
			fifo_throughput_limit TEXT DEFAULT 'perQueue'
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			queue_name TEXT NOT NULL,
			body TEXT NOT NULL,
			attributes TEXT,
			message_attributes TEXT,
			receipt_handle TEXT NOT NULL,
			receive_count INTEGER DEFAULT 0,
			max_receive_count INTEGER DEFAULT 0,
			visibility_timeout DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			delay_seconds INTEGER DEFAULT 0,
			md5_of_body TEXT NOT NULL,
			md5_of_attributes TEXT,
			-- FIFO message fields
			message_group_id TEXT,
			message_deduplication_id TEXT,
			sequence_number TEXT,
			deduplication_hash TEXT,
			FOREIGN KEY (queue_name) REFERENCES queues(name) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_queue_name ON messages(queue_name)`,
		`CREATE INDEX IF NOT EXISTS idx_visibility_timeout ON messages(visibility_timeout)`,
		`CREATE INDEX IF NOT EXISTS idx_receipt_handle ON messages(receipt_handle)`,
		`CREATE INDEX IF NOT EXISTS idx_message_group_id ON messages(queue_name, message_group_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_deduplication_id ON messages(queue_name, message_deduplication_id)`,
		`CREATE INDEX IF NOT EXISTS idx_deduplication_hash ON messages(queue_name, deduplication_hash, created_at)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query %s: %w", query, err)
		}
	}

	return nil
}

func (s *SQLiteStorage) CreateQueue(ctx context.Context, queue *storage.Queue) error {
	attributesJSON, _ := json.Marshal(queue.Attributes)

	query := `INSERT INTO queues (
		name, url, attributes, visibility_timeout_seconds, message_retention_period,
		max_receive_count, delay_seconds, receive_message_wait_time_seconds,
		dead_letter_queue_name, redrive_policy, created_at,
		fifo_queue, content_based_deduplication, deduplication_scope, fifo_throughput_limit
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		queue.Name, queue.URL, string(attributesJSON),
		queue.VisibilityTimeoutSeconds, queue.MessageRetentionPeriod,
		queue.MaxReceiveCount, queue.DelaySeconds, queue.ReceiveMessageWaitTimeSeconds,
		queue.DeadLetterQueueName, queue.RedrivePolicy, queue.CreatedAt,
		queue.FifoQueue, queue.ContentBasedDeduplication, queue.DeduplicationScope, queue.FifoThroughputLimit,
	)

	if err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) CheckForDuplicate(ctx context.Context, queueName, deduplicationHash string, deduplicationWindow time.Duration) (bool, error) {
	cutoffTime := time.Now().Add(-deduplicationWindow)

	query := `SELECT COUNT(*) FROM messages 
		WHERE queue_name = ? AND deduplication_hash = ? AND created_at > ?`

	var count int
	err := s.db.QueryRowContext(ctx, query, queueName, deduplicationHash, cutoffTime).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check for duplicate: %w", err)
	}

	return count > 0, nil
}

func (s *SQLiteStorage) DeleteQueue(ctx context.Context, queueName string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM queues WHERE name = ?", queueName)
	if err != nil {
		return fmt.Errorf("failed to delete queue: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) GetQueue(ctx context.Context, queueName string) (*storage.Queue, error) {
	query := `SELECT name, url, attributes, visibility_timeout_seconds, message_retention_period,
		max_receive_count, delay_seconds, receive_message_wait_time_seconds,
		dead_letter_queue_name, redrive_policy, created_at,
		fifo_queue, content_based_deduplication, deduplication_scope, fifo_throughput_limit
		FROM queues WHERE name = ?`

	row := s.db.QueryRowContext(ctx, query, queueName)

	var queue storage.Queue
	var attributesJSON string
	var deadLetterQueueName, redrivePolicy sql.NullString

	err := row.Scan(
		&queue.Name, &queue.URL, &attributesJSON,
		&queue.VisibilityTimeoutSeconds, &queue.MessageRetentionPeriod,
		&queue.MaxReceiveCount, &queue.DelaySeconds, &queue.ReceiveMessageWaitTimeSeconds,
		&deadLetterQueueName, &redrivePolicy, &queue.CreatedAt,
		&queue.FifoQueue, &queue.ContentBasedDeduplication, &queue.DeduplicationScope, &queue.FifoThroughputLimit,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get queue: %w", err)
	}

	json.Unmarshal([]byte(attributesJSON), &queue.Attributes)

	if deadLetterQueueName.Valid {
		queue.DeadLetterQueueName = deadLetterQueueName.String
	}
	if redrivePolicy.Valid {
		queue.RedrivePolicy = redrivePolicy.String
	}

	return &queue, nil
}

func (s *SQLiteStorage) UpdateQueueAttributes(ctx context.Context, queueName string, attributes map[string]string) error {
	attributesJSON, _ := json.Marshal(attributes)

	query := `UPDATE queues SET attributes = ? WHERE name = ?`
	_, err := s.db.ExecContext(ctx, query, string(attributesJSON), queueName)

	if err != nil {
		return fmt.Errorf("failed to update queue attributes: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) ListQueues(ctx context.Context, prefix string) ([]*storage.Queue, error) {
	query := `SELECT name, url, attributes, created_at FROM queues`
	args := []interface{}{}

	if prefix != "" {
		query += ` WHERE name LIKE ?`
		args = append(args, prefix+"%")
	}

	query += ` ORDER BY name`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list queues: %w", err)
	}
	defer rows.Close()

	var queues []*storage.Queue
	for rows.Next() {
		var queue storage.Queue
		var attributesJSON string

		err := rows.Scan(&queue.Name, &queue.URL, &attributesJSON, &queue.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue: %w", err)
		}

		json.Unmarshal([]byte(attributesJSON), &queue.Attributes)
		queues = append(queues, &queue)
	}

	return queues, nil
}

func (s *SQLiteStorage) SendMessage(ctx context.Context, message *storage.Message) error {
	if message.ID == "" {
		message.ID = uuid.New().String()
	}
	if message.ReceiptHandle == "" {
		message.ReceiptHandle = uuid.New().String()
	}
	if message.MD5OfBody == "" {
		h := md5.Sum([]byte(message.Body))
		message.MD5OfBody = hex.EncodeToString(h[:])
	}

	attributesJSON, _ := json.Marshal(message.Attributes)
	messageAttributesJSON, _ := json.Marshal(message.MessageAttributes)

	query := `INSERT INTO messages (
		id, queue_name, body, attributes, message_attributes, receipt_handle,
		receive_count, max_receive_count, visibility_timeout, created_at,
		delay_seconds, md5_of_body, md5_of_attributes,
		message_group_id, message_deduplication_id, sequence_number, deduplication_hash
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		message.ID, message.QueueName, message.Body,
		string(attributesJSON), string(messageAttributesJSON),
		message.ReceiptHandle, message.ReceiveCount, message.MaxReceiveCount,
		message.VisibilityTimeout, message.CreatedAt,
		message.DelaySeconds, message.MD5OfBody, message.MD5OfAttributes,
		message.MessageGroupId, message.MessageDeduplicationId, message.SequenceNumber, message.DeduplicationHash,
	)

	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) SendMessageBatch(ctx context.Context, messages []*storage.Message) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO messages (
		id, queue_name, body, attributes, message_attributes, receipt_handle,
		receive_count, max_receive_count, visibility_timeout, created_at,
		delay_seconds, md5_of_body, md5_of_attributes
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, message := range messages {
		if message.ID == "" {
			message.ID = uuid.New().String()
		}
		if message.ReceiptHandle == "" {
			message.ReceiptHandle = uuid.New().String()
		}
		if message.MD5OfBody == "" {
			hash := md5.Sum([]byte(message.Body))
			message.MD5OfBody = hex.EncodeToString(hash[:])
		}

		attributesJSON, _ := json.Marshal(message.Attributes)
		messageAttributesJSON, _ := json.Marshal(message.MessageAttributes)

		_, err = stmt.ExecContext(ctx,
			message.ID, message.QueueName, message.Body,
			string(attributesJSON), string(messageAttributesJSON),
			message.ReceiptHandle, message.ReceiveCount, message.MaxReceiveCount,
			message.VisibilityTimeout, message.CreatedAt,
			message.DelaySeconds, message.MD5OfBody, message.MD5OfAttributes,
		)
		if err != nil {
			return fmt.Errorf("failed to insert batch message: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) ReceiveMessages(ctx context.Context, queueName string, maxMessages int, waitTimeSeconds int, visibilityTimeout int) ([]*storage.Message, error) {
	now := time.Now()

	// Use a transaction to ensure consistency
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if this is a FIFO queue by examining queue table
	var isFifoQueue bool
	err = tx.QueryRowContext(ctx, "SELECT fifo_queue FROM queues WHERE name = ?", queueName).Scan(&isFifoQueue)
	if err != nil {
		return nil, fmt.Errorf("failed to check queue type: %w", err)
	}

	// Select messages that are available (not in visibility timeout)
	var query string
	if isFifoQueue {
		// For FIFO queues: order by message_group_id first, then by sequence_number for proper FIFO ordering
		query = `SELECT id, queue_name, body, attributes, message_attributes, receipt_handle,
			receive_count, max_receive_count, visibility_timeout, created_at,
			delay_seconds, md5_of_body, md5_of_attributes,
			message_group_id, message_deduplication_id, sequence_number, deduplication_hash
			FROM messages 
			WHERE queue_name = ? AND (visibility_timeout IS NULL OR visibility_timeout <= ?)
			ORDER BY message_group_id ASC, sequence_number ASC
			LIMIT ?`
	} else {
		// For standard queues: order by created_at (existing behavior)
		query = `SELECT id, queue_name, body, attributes, message_attributes, receipt_handle,
			receive_count, max_receive_count, visibility_timeout, created_at,
			delay_seconds, md5_of_body, md5_of_attributes,
			message_group_id, message_deduplication_id, sequence_number, deduplication_hash
			FROM messages 
			WHERE queue_name = ? AND (visibility_timeout IS NULL OR visibility_timeout <= ?)
			ORDER BY created_at ASC
			LIMIT ?`
	}

	rows, err := tx.QueryContext(ctx, query, queueName, now, maxMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*storage.Message
	var messageIDs []string
	var currentMessageGroupId string

	for rows.Next() {
		var message storage.Message
		var attributesJSON, messageAttributesJSON sql.NullString
		var dbVisibilityTimeout sql.NullTime
		var messageGroupId, messageDeduplicationId, sequenceNumber, deduplicationHash sql.NullString

		err := rows.Scan(
			&message.ID, &message.QueueName, &message.Body,
			&attributesJSON, &messageAttributesJSON, &message.ReceiptHandle,
			&message.ReceiveCount, &message.MaxReceiveCount, &dbVisibilityTimeout,
			&message.CreatedAt, &message.DelaySeconds, &message.MD5OfBody, &message.MD5OfAttributes,
			&messageGroupId, &messageDeduplicationId, &sequenceNumber, &deduplicationHash,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		if attributesJSON.Valid {
			json.Unmarshal([]byte(attributesJSON.String), &message.Attributes)
		}
		if messageAttributesJSON.Valid {
			json.Unmarshal([]byte(messageAttributesJSON.String), &message.MessageAttributes)
		}
		if dbVisibilityTimeout.Valid {
			message.VisibilityTimeout = dbVisibilityTimeout.Time
		}

		// Assign FIFO fields
		if messageGroupId.Valid {
			message.MessageGroupId = messageGroupId.String
		}
		if messageDeduplicationId.Valid {
			message.MessageDeduplicationId = messageDeduplicationId.String
		}
		if sequenceNumber.Valid {
			message.SequenceNumber = sequenceNumber.String
		}
		if deduplicationHash.Valid {
			message.DeduplicationHash = deduplicationHash.String
		}

		// For FIFO queues, ensure we only return messages from the same message group
		// to maintain strict ordering within the group
		if isFifoQueue {
			if currentMessageGroupId == "" {
				// First message - set the message group
				currentMessageGroupId = message.MessageGroupId
			} else if currentMessageGroupId != message.MessageGroupId {
				// Different message group - stop here to maintain ordering
				break
			}
		}

		// Update for return
		message.ReceiveCount++
		message.ReceiptHandle = uuid.New().String()

		// Use the provided visibility timeout or queue default (30 seconds)
		timeoutDuration := 30
		if visibilityTimeout > 0 {
			timeoutDuration = visibilityTimeout
		}
		message.VisibilityTimeout = now.Add(time.Duration(timeoutDuration) * time.Second)

		messages = append(messages, &message)
		messageIDs = append(messageIDs, message.ID)
	}
	rows.Close()

	// Update all messages in a batch
	if len(messageIDs) > 0 {
		for i, message := range messages {
			_, err = tx.ExecContext(ctx,
				"UPDATE messages SET receive_count = ?, receipt_handle = ?, visibility_timeout = ? WHERE id = ?",
				message.ReceiveCount, message.ReceiptHandle, message.VisibilityTimeout, messageIDs[i],
			)
			if err != nil {
				return nil, fmt.Errorf("failed to update message visibility: %w", err)
			}
		}
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return messages, nil
}

func (s *SQLiteStorage) DeleteMessage(ctx context.Context, queueName string, receiptHandle string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM messages WHERE queue_name = ? AND receipt_handle = ?",
		queueName, receiptHandle)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) DeleteMessageBatch(ctx context.Context, queueName string, receiptHandles []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, handle := range receiptHandles {
		if err := s.DeleteMessage(ctx, queueName, handle); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) ChangeMessageVisibility(ctx context.Context, queueName string, receiptHandle string, visibilityTimeout int) error {
	newVisibility := time.Now().Add(time.Duration(visibilityTimeout) * time.Second)
	_, err := s.db.ExecContext(ctx,
		"UPDATE messages SET visibility_timeout = ? WHERE queue_name = ? AND receipt_handle = ?",
		newVisibility, queueName, receiptHandle)
	if err != nil {
		return fmt.Errorf("failed to change message visibility: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) ChangeMessageVisibilityBatch(ctx context.Context, queueName string, entries []storage.VisibilityEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Use a transaction for batch operations
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	stmt, err := tx.PrepareContext(ctx, "UPDATE messages SET visibility_timeout = ? WHERE queue_name = ? AND receipt_handle = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, entry := range entries {
		newVisibility := now.Add(time.Duration(entry.VisibilityTimeout) * time.Second)
		_, err := stmt.ExecContext(ctx, newVisibility, queueName, entry.ReceiptHandle)
		if err != nil {
			return fmt.Errorf("failed to update visibility for receipt handle %s: %w", entry.ReceiptHandle, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch visibility update: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) MoveMessageToDLQ(ctx context.Context, message *storage.Message, dlqName string) error {
	dlqMessage := *message
	dlqMessage.QueueName = dlqName
	dlqMessage.ID = uuid.New().String()
	dlqMessage.ReceiptHandle = uuid.New().String()
	dlqMessage.ReceiveCount = 0
	dlqMessage.VisibilityTimeout = time.Time{}

	if err := s.SendMessage(context.Background(), &dlqMessage); err != nil {
		return fmt.Errorf("failed to send message to DLQ: %w", err)
	}

	return s.DeleteMessage(ctx, message.QueueName, message.ReceiptHandle)
}

func (s *SQLiteStorage) GetExpiredMessages(ctx context.Context) ([]*storage.Message, error) {
	query := `SELECT id, queue_name, body, attributes, message_attributes, receipt_handle,
		receive_count, max_receive_count, visibility_timeout, created_at,
		delay_seconds, md5_of_body, md5_of_attributes
		FROM messages 
		WHERE receive_count >= max_receive_count AND max_receive_count > 0`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get expired messages: %w", err)
	}
	defer rows.Close()

	var messages []*storage.Message
	for rows.Next() {
		var message storage.Message
		var attributesJSON, messageAttributesJSON sql.NullString
		var visibilityTimeout sql.NullTime

		err := rows.Scan(
			&message.ID, &message.QueueName, &message.Body,
			&attributesJSON, &messageAttributesJSON, &message.ReceiptHandle,
			&message.ReceiveCount, &message.MaxReceiveCount, &visibilityTimeout,
			&message.CreatedAt, &message.DelaySeconds, &message.MD5OfBody, &message.MD5OfAttributes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan expired message: %w", err)
		}

		if attributesJSON.Valid {
			json.Unmarshal([]byte(attributesJSON.String), &message.Attributes)
		}
		if messageAttributesJSON.Valid {
			json.Unmarshal([]byte(messageAttributesJSON.String), &message.MessageAttributes)
		}
		if visibilityTimeout.Valid {
			message.VisibilityTimeout = visibilityTimeout.Time
		}

		messages = append(messages, &message)
	}

	return messages, nil
}

func (s *SQLiteStorage) GetInFlightMessages(ctx context.Context, queueName string) ([]*storage.Message, error) {
	query := `SELECT id, queue_name, body, attributes, message_attributes, receipt_handle,
		receive_count, max_receive_count, visibility_timeout, created_at,
		delay_seconds, md5_of_body, md5_of_attributes
		FROM messages WHERE queue_name = ?`

	rows, err := s.db.QueryContext(ctx, query, queueName)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages for queue %s: %w", queueName, err)
	}
	defer rows.Close()

	var messages []*storage.Message
	for rows.Next() {
		var message storage.Message
		var attributesJSON, messageAttributesJSON sql.NullString
		var visibilityTimeout sql.NullTime

		err := rows.Scan(
			&message.ID, &message.QueueName, &message.Body,
			&attributesJSON, &messageAttributesJSON, &message.ReceiptHandle,
			&message.ReceiveCount, &message.MaxReceiveCount, &visibilityTimeout,
			&message.CreatedAt, &message.DelaySeconds, &message.MD5OfBody, &message.MD5OfAttributes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		if attributesJSON.Valid {
			json.Unmarshal([]byte(attributesJSON.String), &message.Attributes)
		}
		if messageAttributesJSON.Valid {
			json.Unmarshal([]byte(messageAttributesJSON.String), &message.MessageAttributes)
		}
		if visibilityTimeout.Valid {
			message.VisibilityTimeout = visibilityTimeout.Time
			message.VisibleAt = &visibilityTimeout.Time
		}

		messages = append(messages, &message)
	}

	return messages, nil
}

func (s *SQLiteStorage) PurgeQueue(ctx context.Context, queueName string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM messages WHERE queue_name = ?", queueName)
	if err != nil {
		return fmt.Errorf("failed to purge queue: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
