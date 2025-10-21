package postgres

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"sqs-bridge/src/storage"
)

type PostgreSQLStorage struct {
	db     *sql.DB
	schema string
}

func NewPostgreSQLStorage(databaseURL, host, port, user, password, dbname, schema string) (*PostgreSQLStorage, error) {
	var connStr string

	if databaseURL != "" {
		connStr = databaseURL
	} else {
		connStr = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			host, port, user, password, dbname)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if schema == "" {
		schema = "sqsbridge"
	}

	s := &PostgreSQLStorage{
		db:     db,
		schema: schema,
	}

	if err := s.createSchema(); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	if err := s.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return s, nil
}

func (s *PostgreSQLStorage) createSchema() error {
	query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", pq.QuoteIdentifier(s.schema))
	_, err := s.db.Exec(query)
	return err
}

func (s *PostgreSQLStorage) createTables() error {
	queries := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.queues (
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
			created_at TIMESTAMPTZ DEFAULT NOW(),
			fifo_queue BOOLEAN DEFAULT FALSE,
			content_based_deduplication BOOLEAN DEFAULT FALSE,
			deduplication_scope TEXT DEFAULT 'queue',
			fifo_throughput_limit TEXT DEFAULT 'perQueue'
		)`, pq.QuoteIdentifier(s.schema)),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.messages (
			id TEXT PRIMARY KEY,
			queue_name TEXT NOT NULL,
			body TEXT NOT NULL,
			attributes TEXT,
			message_attributes TEXT,
			receipt_handle TEXT NOT NULL,
			receive_count INTEGER DEFAULT 0,
			max_receive_count INTEGER DEFAULT 0,
			visibility_timeout TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			delay_seconds INTEGER DEFAULT 0,
			md5_of_body TEXT NOT NULL,
			md5_of_attributes TEXT,
			message_group_id TEXT,
			message_deduplication_id TEXT,
			sequence_number TEXT,
			deduplication_hash TEXT,
			FOREIGN KEY (queue_name) REFERENCES %s.queues(name) ON DELETE CASCADE
		)`, pq.QuoteIdentifier(s.schema), pq.QuoteIdentifier(s.schema)),

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.access_keys (
			access_key_id TEXT PRIMARY KEY,
			secret_access_key TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			last_used_at TIMESTAMPTZ
		)`, pq.QuoteIdentifier(s.schema)),

		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_queue_name ON %s.messages(queue_name)`, pq.QuoteIdentifier(s.schema)),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_visibility_timeout ON %s.messages(visibility_timeout)`, pq.QuoteIdentifier(s.schema)),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_receipt_handle ON %s.messages(receipt_handle)`, pq.QuoteIdentifier(s.schema)),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_message_group_id ON %s.messages(queue_name, message_group_id, created_at)`, pq.QuoteIdentifier(s.schema)),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_deduplication_id ON %s.messages(queue_name, message_deduplication_id)`, pq.QuoteIdentifier(s.schema)),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_deduplication_hash ON %s.messages(queue_name, deduplication_hash, created_at)`, pq.QuoteIdentifier(s.schema)),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_access_key_active ON %s.access_keys(active)`, pq.QuoteIdentifier(s.schema)),
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query %s: %w", query, err)
		}
	}

	return nil
}

func (s *PostgreSQLStorage) CreateQueue(ctx context.Context, queue *storage.Queue) error {
	attributesJSON, err := json.Marshal(queue.Attributes)
	if err != nil {
		return fmt.Errorf("failed to marshal attributes: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s.queues (
			name, url, attributes, visibility_timeout_seconds, message_retention_period, 
			max_receive_count, delay_seconds, receive_message_wait_time_seconds, 
			dead_letter_queue_name, redrive_policy, fifo_queue, content_based_deduplication,
			deduplication_scope, fifo_throughput_limit
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (name) DO UPDATE SET
			url = EXCLUDED.url,
			attributes = EXCLUDED.attributes,
			visibility_timeout_seconds = EXCLUDED.visibility_timeout_seconds,
			message_retention_period = EXCLUDED.message_retention_period,
			max_receive_count = EXCLUDED.max_receive_count,
			delay_seconds = EXCLUDED.delay_seconds,
			receive_message_wait_time_seconds = EXCLUDED.receive_message_wait_time_seconds,
			dead_letter_queue_name = EXCLUDED.dead_letter_queue_name,
			redrive_policy = EXCLUDED.redrive_policy,
			fifo_queue = EXCLUDED.fifo_queue,
			content_based_deduplication = EXCLUDED.content_based_deduplication,
			deduplication_scope = EXCLUDED.deduplication_scope,
			fifo_throughput_limit = EXCLUDED.fifo_throughput_limit
	`, pq.QuoteIdentifier(s.schema))

	_, err = s.db.ExecContext(ctx, query,
		queue.Name, queue.URL, string(attributesJSON), queue.VisibilityTimeoutSeconds,
		queue.MessageRetentionPeriod, queue.MaxReceiveCount, queue.DelaySeconds,
		queue.ReceiveMessageWaitTimeSeconds, queue.DeadLetterQueueName, queue.RedrivePolicy,
		queue.FifoQueue, queue.ContentBasedDeduplication, queue.DeduplicationScope,
		queue.FifoThroughputLimit)

	return err
}

func (s *PostgreSQLStorage) GetQueue(ctx context.Context, queueName string) (*storage.Queue, error) {
	query := fmt.Sprintf(`
		SELECT name, url, attributes, visibility_timeout_seconds, message_retention_period,
		       max_receive_count, delay_seconds, receive_message_wait_time_seconds,
		       dead_letter_queue_name, redrive_policy, created_at, fifo_queue,
		       content_based_deduplication, deduplication_scope, fifo_throughput_limit
		FROM %s.queues WHERE name = $1
	`, pq.QuoteIdentifier(s.schema))

	var queue storage.Queue
	var attributesJSON string
	var createdAt time.Time

	err := s.db.QueryRowContext(ctx, query, queueName).Scan(
		&queue.Name, &queue.URL, &attributesJSON, &queue.VisibilityTimeoutSeconds,
		&queue.MessageRetentionPeriod, &queue.MaxReceiveCount, &queue.DelaySeconds,
		&queue.ReceiveMessageWaitTimeSeconds, &queue.DeadLetterQueueName, &queue.RedrivePolicy,
		&createdAt, &queue.FifoQueue, &queue.ContentBasedDeduplication,
		&queue.DeduplicationScope, &queue.FifoThroughputLimit)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(attributesJSON), &queue.Attributes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal attributes: %w", err)
	}

	queue.CreatedAt = createdAt
	return &queue, nil
}

func (s *PostgreSQLStorage) ListQueues(ctx context.Context, prefix string) ([]*storage.Queue, error) {
	query := fmt.Sprintf(`
		SELECT name, url, attributes, visibility_timeout_seconds, message_retention_period,
		       max_receive_count, delay_seconds, receive_message_wait_time_seconds,
		       dead_letter_queue_name, redrive_policy, created_at, fifo_queue,
		       content_based_deduplication, deduplication_scope, fifo_throughput_limit
		FROM %s.queues`, pq.QuoteIdentifier(s.schema))

	args := []interface{}{}
	if prefix != "" {
		query += ` WHERE name LIKE $1`
		args = append(args, prefix+"%")
	}
	query += ` ORDER BY name`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queues []*storage.Queue
	for rows.Next() {
		var queue storage.Queue
		var attributesJSON string
		var createdAt time.Time

		err := rows.Scan(
			&queue.Name, &queue.URL, &attributesJSON, &queue.VisibilityTimeoutSeconds,
			&queue.MessageRetentionPeriod, &queue.MaxReceiveCount, &queue.DelaySeconds,
			&queue.ReceiveMessageWaitTimeSeconds, &queue.DeadLetterQueueName, &queue.RedrivePolicy,
			&createdAt, &queue.FifoQueue, &queue.ContentBasedDeduplication,
			&queue.DeduplicationScope, &queue.FifoThroughputLimit)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(attributesJSON), &queue.Attributes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal attributes: %w", err)
		}

		queue.CreatedAt = createdAt
		queues = append(queues, &queue)
	}

	return queues, rows.Err()
}

func (s *PostgreSQLStorage) DeleteQueue(ctx context.Context, queueName string) error {
	query := fmt.Sprintf(`DELETE FROM %s.queues WHERE name = $1`, pq.QuoteIdentifier(s.schema))
	_, err := s.db.ExecContext(ctx, query, queueName)
	return err
}

func (s *PostgreSQLStorage) SendMessage(ctx context.Context, message *storage.Message) error {
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

	attributesJSON := "{}"
	if message.Attributes != nil {
		if data, err := json.Marshal(message.Attributes); err == nil {
			attributesJSON = string(data)
		}
	}

	messageAttributesJSON := "{}"
	if message.MessageAttributes != nil {
		if data, err := json.Marshal(message.MessageAttributes); err == nil {
			messageAttributesJSON = string(data)
		}
	}

	query := fmt.Sprintf(`
		INSERT INTO %s.messages (
			id, queue_name, body, attributes, message_attributes, receipt_handle,
			receive_count, max_receive_count, visibility_timeout, delay_seconds,
			md5_of_body, md5_of_attributes, message_group_id, message_deduplication_id,
			sequence_number, deduplication_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, pq.QuoteIdentifier(s.schema))

	_, err := s.db.ExecContext(ctx, query,
		message.ID, message.QueueName, message.Body, attributesJSON, messageAttributesJSON,
		message.ReceiptHandle, message.ReceiveCount, message.MaxReceiveCount,
		message.VisibilityTimeout, message.DelaySeconds, message.MD5OfBody,
		message.MD5OfAttributes, message.MessageGroupId, message.MessageDeduplicationId,
		message.SequenceNumber, message.DeduplicationHash)

	return err
}

func (s *PostgreSQLStorage) ReceiveMessages(ctx context.Context, queueName string, maxMessages int, waitTimeSeconds int, visibilityTimeout int) ([]*storage.Message, error) {
	now := time.Now()

	// Use a transaction to ensure consistency
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if this is a FIFO queue by examining queue table
	var isFifoQueue bool
	err = tx.QueryRowContext(ctx, fmt.Sprintf("SELECT fifo_queue FROM %s.queues WHERE name = $1", pq.QuoteIdentifier(s.schema)), queueName).Scan(&isFifoQueue)
	if err != nil {
		return nil, fmt.Errorf("failed to check queue type: %w", err)
	}

	// Select messages that are available (not in visibility timeout)
	var query string
	if isFifoQueue {
		// For FIFO queues: order by message_group_id first, then by sequence_number for proper FIFO ordering
		query = fmt.Sprintf(`
			SELECT id, queue_name, body, attributes, message_attributes, receipt_handle,
			       receive_count, max_receive_count, visibility_timeout, created_at,
			       delay_seconds, md5_of_body, md5_of_attributes,
			       message_group_id, message_deduplication_id, sequence_number, deduplication_hash
			FROM %s.messages 
			WHERE queue_name = $1 AND (visibility_timeout IS NULL OR visibility_timeout <= $2)
			      AND (delay_seconds = 0 OR created_at + INTERVAL '1 second' * delay_seconds <= $2)
			ORDER BY message_group_id ASC, sequence_number ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		`, pq.QuoteIdentifier(s.schema))
	} else {
		// For standard queues: order by created_at (existing behavior)
		query = fmt.Sprintf(`
			SELECT id, queue_name, body, attributes, message_attributes, receipt_handle,
			       receive_count, max_receive_count, visibility_timeout, created_at,
			       delay_seconds, md5_of_body, md5_of_attributes,
			       message_group_id, message_deduplication_id, sequence_number, deduplication_hash
			FROM %s.messages 
			WHERE queue_name = $1 AND (visibility_timeout IS NULL OR visibility_timeout <= $2)
			      AND (delay_seconds = 0 OR created_at + INTERVAL '1 second' * delay_seconds <= $2)
			ORDER BY created_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		`, pq.QuoteIdentifier(s.schema))
	}

	rows, err := tx.QueryContext(ctx, query, queueName, now, maxMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*storage.Message
	var messageIDs []string

	for rows.Next() {
		var message storage.Message
		var attributesJSON, messageAttributesJSON string
		var dbVisibilityTimeout sql.NullTime
		var messageGroupId, messageDeduplicationId, sequenceNumber, deduplicationHash sql.NullString
		var createdAt time.Time

		err := rows.Scan(
			&message.ID, &message.QueueName, &message.Body,
			&attributesJSON, &messageAttributesJSON, &message.ReceiptHandle,
			&message.ReceiveCount, &message.MaxReceiveCount, &dbVisibilityTimeout,
			&createdAt, &message.DelaySeconds, &message.MD5OfBody, &message.MD5OfAttributes,
			&messageGroupId, &messageDeduplicationId, &sequenceNumber, &deduplicationHash,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		if attributesJSON != "" {
			json.Unmarshal([]byte(attributesJSON), &message.Attributes)
		}
		if messageAttributesJSON != "" {
			json.Unmarshal([]byte(messageAttributesJSON), &message.MessageAttributes)
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

		// Update for return
		message.ReceiveCount++
		message.ReceiptHandle = uuid.New().String()

		// Use the provided visibility timeout or queue default (30 seconds)
		timeoutDuration := 30
		if visibilityTimeout > 0 {
			timeoutDuration = visibilityTimeout
		}
		message.VisibilityTimeout = now.Add(time.Duration(timeoutDuration) * time.Second)
		message.CreatedAt = createdAt

		messages = append(messages, &message)
		messageIDs = append(messageIDs, message.ID)
	}
	rows.Close()

	// Update all messages in a batch
	if len(messageIDs) > 0 {
		for i, message := range messages {
			_, err = tx.ExecContext(ctx, fmt.Sprintf(
				"UPDATE %s.messages SET receive_count = $1, receipt_handle = $2, visibility_timeout = $3 WHERE id = $4",
				pq.QuoteIdentifier(s.schema)),
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

func (s *PostgreSQLStorage) DeleteMessage(ctx context.Context, queueName, receiptHandle string) error {
	query := fmt.Sprintf(`DELETE FROM %s.messages WHERE queue_name = $1 AND receipt_handle = $2`, pq.QuoteIdentifier(s.schema))
	result, err := s.db.ExecContext(ctx, query, queueName, receiptHandle)
	if err != nil {
		return err
	}

	// Check if any rows were actually deleted
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("receipt handle not found")
	}

	return nil
}

func (s *PostgreSQLStorage) DeleteMessageBatch(ctx context.Context, queueName string, receiptHandles []string) error {
	if len(receiptHandles) == 0 {
		return nil
	}

	query := fmt.Sprintf(`DELETE FROM %s.messages WHERE queue_name = $1 AND receipt_handle = ANY($2)`, pq.QuoteIdentifier(s.schema))
	_, err := s.db.ExecContext(ctx, query, queueName, pq.Array(receiptHandles))
	return err
}

func (s *PostgreSQLStorage) ChangeMessageVisibility(ctx context.Context, queueName, receiptHandle string, visibilityTimeout int) error {
	newVisibilityTimeout := time.Now().Add(time.Duration(visibilityTimeout) * time.Second)
	query := fmt.Sprintf(`
		UPDATE %s.messages 
		SET visibility_timeout = $1 
		WHERE queue_name = $2 AND receipt_handle = $3
	`, pq.QuoteIdentifier(s.schema))

	_, err := s.db.ExecContext(ctx, query, newVisibilityTimeout, queueName, receiptHandle)
	return err
}

func (s *PostgreSQLStorage) PurgeQueue(ctx context.Context, queueName string) error {
	query := fmt.Sprintf(`DELETE FROM %s.messages WHERE queue_name = $1`, pq.QuoteIdentifier(s.schema))
	_, err := s.db.ExecContext(ctx, query, queueName)
	return err
}

func (s *PostgreSQLStorage) GetExpiredMessages(ctx context.Context) ([]*storage.Message, error) {
	query := fmt.Sprintf(`
		SELECT id, queue_name, body, attributes, message_attributes, receipt_handle,
		       receive_count, max_receive_count, visibility_timeout, created_at,
		       delay_seconds, md5_of_body, md5_of_attributes, message_group_id,
		       message_deduplication_id, sequence_number, deduplication_hash
		FROM %s.messages
		WHERE receive_count >= max_receive_count AND max_receive_count > 0
	`, pq.QuoteIdentifier(s.schema))

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*storage.Message
	for rows.Next() {
		var message storage.Message
		var attributesJSON, messageAttributesJSON string
		var createdAt time.Time

		err := rows.Scan(
			&message.ID, &message.QueueName, &message.Body, &attributesJSON,
			&messageAttributesJSON, &message.ReceiptHandle, &message.ReceiveCount,
			&message.MaxReceiveCount, &message.VisibilityTimeout, &createdAt,
			&message.DelaySeconds, &message.MD5OfBody, &message.MD5OfAttributes,
			&message.MessageGroupId, &message.MessageDeduplicationId,
			&message.SequenceNumber, &message.DeduplicationHash)
		if err != nil {
			return nil, err
		}

		if attributesJSON != "" {
			json.Unmarshal([]byte(attributesJSON), &message.Attributes)
		}
		if messageAttributesJSON != "" {
			json.Unmarshal([]byte(messageAttributesJSON), &message.MessageAttributes)
		}

		message.CreatedAt = createdAt
		messages = append(messages, &message)
	}

	return messages, rows.Err()
}

func (s *PostgreSQLStorage) MoveMessageToDLQ(ctx context.Context, message *storage.Message, dlqName string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert into DLQ
	insertQuery := fmt.Sprintf(`
		INSERT INTO %s.messages (
			id, queue_name, body, attributes, message_attributes, receipt_handle,
			receive_count, max_receive_count, visibility_timeout, delay_seconds,
			md5_of_body, md5_of_attributes, message_group_id, message_deduplication_id,
			sequence_number, deduplication_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, pq.QuoteIdentifier(s.schema))

	attributesJSON := "{}"
	if message.Attributes != nil {
		if data, err := json.Marshal(message.Attributes); err == nil {
			attributesJSON = string(data)
		}
	}

	messageAttributesJSON := "{}"
	if message.MessageAttributes != nil {
		if data, err := json.Marshal(message.MessageAttributes); err == nil {
			messageAttributesJSON = string(data)
		}
	}

	_, err = tx.ExecContext(ctx, insertQuery,
		message.ID+"-dlq", dlqName, message.Body, attributesJSON, messageAttributesJSON,
		message.ReceiptHandle+"-dlq", 0, 0, nil, message.DelaySeconds,
		message.MD5OfBody, message.MD5OfAttributes, message.MessageGroupId,
		message.MessageDeduplicationId, message.SequenceNumber, message.DeduplicationHash)
	if err != nil {
		return err
	}

	// Delete from original queue
	deleteQuery := fmt.Sprintf(`DELETE FROM %s.messages WHERE id = $1`, pq.QuoteIdentifier(s.schema))
	_, err = tx.ExecContext(ctx, deleteQuery, message.ID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *PostgreSQLStorage) ChangeMessageVisibilityBatch(ctx context.Context, queueName string, entries []storage.VisibilityEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		UPDATE %s.messages 
		SET visibility_timeout = $1 
		WHERE queue_name = $2 AND receipt_handle = $3
	`, pq.QuoteIdentifier(s.schema)))
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

func (s *PostgreSQLStorage) SendMessageBatch(ctx context.Context, messages []*storage.Message) error {
	if len(messages) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		INSERT INTO %s.messages (
			id, queue_name, body, attributes, message_attributes, receipt_handle,
			receive_count, max_receive_count, visibility_timeout, created_at,
			delay_seconds, md5_of_body, md5_of_attributes, message_group_id,
			message_deduplication_id, sequence_number, deduplication_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, pq.QuoteIdentifier(s.schema)))
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
			message.MessageGroupId, message.MessageDeduplicationId,
			message.SequenceNumber, message.DeduplicationHash,
		)
		if err != nil {
			return fmt.Errorf("failed to insert batch message: %w", err)
		}
	}

	return tx.Commit()
}

func (s *PostgreSQLStorage) UpdateQueueAttributes(ctx context.Context, queueName string, attributes map[string]string) error {
	// Build SET clauses for attributes that have dedicated columns
	setClauses := []string{}
	args := []interface{}{}
	argIndex := 1

	// Update dedicated columns based on attributes
	if val, ok := attributes["VisibilityTimeout"]; ok {
		if timeout, err := strconv.Atoi(val); err == nil {
			setClauses = append(setClauses, fmt.Sprintf("visibility_timeout_seconds = $%d", argIndex))
			args = append(args, timeout)
			argIndex++
		}
	}

	if val, ok := attributes["MessageRetentionPeriod"]; ok {
		if period, err := strconv.Atoi(val); err == nil {
			setClauses = append(setClauses, fmt.Sprintf("message_retention_period = $%d", argIndex))
			args = append(args, period)
			argIndex++
		}
	}

	if val, ok := attributes["DelaySeconds"]; ok {
		if delay, err := strconv.Atoi(val); err == nil {
			setClauses = append(setClauses, fmt.Sprintf("delay_seconds = $%d", argIndex))
			args = append(args, delay)
			argIndex++
		}
	}

	if val, ok := attributes["MaxReceiveCount"]; ok {
		if count, err := strconv.Atoi(val); err == nil {
			setClauses = append(setClauses, fmt.Sprintf("max_receive_count = $%d", argIndex))
			args = append(args, count)
			argIndex++
		}
	}

	// Always update the attributes JSON
	attributesJSON, _ := json.Marshal(attributes)
	setClauses = append(setClauses, fmt.Sprintf("attributes = $%d", argIndex))
	args = append(args, string(attributesJSON))
	argIndex++

	// Add queue name for WHERE clause
	args = append(args, queueName)

	if len(setClauses) == 0 {
		return nil // Nothing to update
	}

	query := fmt.Sprintf(`UPDATE %s.queues SET %s WHERE name = $%d`,
		pq.QuoteIdentifier(s.schema),
		strings.Join(setClauses, ", "),
		argIndex)

	_, err := s.db.ExecContext(ctx, query, args...)

	if err != nil {
		return fmt.Errorf("failed to update queue attributes: %w", err)
	}

	return nil
}

func (s *PostgreSQLStorage) CheckForDuplicate(ctx context.Context, queueName, deduplicationHash string, deduplicationWindow time.Duration) (bool, error) {
	cutoffTime := time.Now().Add(-deduplicationWindow)

	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM %s.messages 
		WHERE queue_name = $1 AND deduplication_hash = $2 AND created_at > $3
	`, pq.QuoteIdentifier(s.schema))

	var count int
	err := s.db.QueryRowContext(ctx, query, queueName, deduplicationHash, cutoffTime).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check for duplicate: %w", err)
	}

	return count > 0, nil
}

func (s *PostgreSQLStorage) GetInFlightMessages(ctx context.Context, queueName string) ([]*storage.Message, error) {
	query := fmt.Sprintf(`
		SELECT id, queue_name, body, attributes, message_attributes, receipt_handle,
		       receive_count, max_receive_count, visibility_timeout, created_at,
		       delay_seconds, md5_of_body, md5_of_attributes, message_group_id,
		       message_deduplication_id, sequence_number, deduplication_hash
		FROM %s.messages WHERE queue_name = $1
	`, pq.QuoteIdentifier(s.schema))

	rows, err := s.db.QueryContext(ctx, query, queueName)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages for queue %s: %w", queueName, err)
	}
	defer rows.Close()

	var messages []*storage.Message
	for rows.Next() {
		var message storage.Message
		var attributesJSON, messageAttributesJSON string
		var visibilityTimeout sql.NullTime
		var createdAt time.Time

		err := rows.Scan(
			&message.ID, &message.QueueName, &message.Body, &attributesJSON,
			&messageAttributesJSON, &message.ReceiptHandle, &message.ReceiveCount,
			&message.MaxReceiveCount, &visibilityTimeout, &createdAt,
			&message.DelaySeconds, &message.MD5OfBody, &message.MD5OfAttributes,
			&message.MessageGroupId, &message.MessageDeduplicationId,
			&message.SequenceNumber, &message.DeduplicationHash)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		if attributesJSON != "" {
			json.Unmarshal([]byte(attributesJSON), &message.Attributes)
		}
		if messageAttributesJSON != "" {
			json.Unmarshal([]byte(messageAttributesJSON), &message.MessageAttributes)
		}
		if visibilityTimeout.Valid {
			message.VisibilityTimeout = visibilityTimeout.Time
			message.VisibleAt = &visibilityTimeout.Time
		}

		message.CreatedAt = createdAt
		messages = append(messages, &message)
	}

	return messages, nil
}

func (s *PostgreSQLStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Access Key Operations
func (s *PostgreSQLStorage) CreateAccessKey(ctx context.Context, accessKey *storage.AccessKey) error {
	query := fmt.Sprintf(`INSERT INTO %s.access_keys (
		access_key_id, secret_access_key, name, description, active, created_at, last_used_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7)`, pq.QuoteIdentifier(s.schema))

	_, err := s.db.ExecContext(ctx, query,
		accessKey.AccessKeyID, accessKey.SecretAccessKey, accessKey.Name,
		accessKey.Description, accessKey.Active, accessKey.CreatedAt, accessKey.LastUsedAt)

	if err != nil {
		return fmt.Errorf("failed to create access key: %w", err)
	}

	return nil
}

func (s *PostgreSQLStorage) GetAccessKey(ctx context.Context, accessKeyID string) (*storage.AccessKey, error) {
	query := fmt.Sprintf(`SELECT access_key_id, secret_access_key, name, description, active, created_at, last_used_at 
		FROM %s.access_keys WHERE access_key_id = $1`, pq.QuoteIdentifier(s.schema))

	row := s.db.QueryRowContext(ctx, query, accessKeyID)

	var accessKey storage.AccessKey
	var lastUsedAt sql.NullTime

	err := row.Scan(
		&accessKey.AccessKeyID, &accessKey.SecretAccessKey, &accessKey.Name,
		&accessKey.Description, &accessKey.Active, &accessKey.CreatedAt, &lastUsedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get access key: %w", err)
	}

	if lastUsedAt.Valid {
		accessKey.LastUsedAt = &lastUsedAt.Time
	}

	return &accessKey, nil
}

func (s *PostgreSQLStorage) ListAccessKeys(ctx context.Context) ([]*storage.AccessKey, error) {
	query := fmt.Sprintf(`SELECT access_key_id, secret_access_key, name, description, active, created_at, last_used_at 
		FROM %s.access_keys ORDER BY created_at DESC`, pq.QuoteIdentifier(s.schema))

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list access keys: %w", err)
	}
	defer rows.Close()

	var accessKeys []*storage.AccessKey
	for rows.Next() {
		var accessKey storage.AccessKey
		var lastUsedAt sql.NullTime

		err := rows.Scan(
			&accessKey.AccessKeyID, &accessKey.SecretAccessKey, &accessKey.Name,
			&accessKey.Description, &accessKey.Active, &accessKey.CreatedAt, &lastUsedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan access key: %w", err)
		}

		if lastUsedAt.Valid {
			accessKey.LastUsedAt = &lastUsedAt.Time
		}

		accessKeys = append(accessKeys, &accessKey)
	}

	return accessKeys, nil
}

func (s *PostgreSQLStorage) DeactivateAccessKey(ctx context.Context, accessKeyID string) error {
	query := fmt.Sprintf("UPDATE %s.access_keys SET active = FALSE WHERE access_key_id = $1", pq.QuoteIdentifier(s.schema))
	_, err := s.db.ExecContext(ctx, query, accessKeyID)
	if err != nil {
		return fmt.Errorf("failed to deactivate access key: %w", err)
	}
	return nil
}

func (s *PostgreSQLStorage) DeleteAccessKey(ctx context.Context, accessKeyID string) error {
	query := fmt.Sprintf("DELETE FROM %s.access_keys WHERE access_key_id = $1", pq.QuoteIdentifier(s.schema))
	_, err := s.db.ExecContext(ctx, query, accessKeyID)
	if err != nil {
		return fmt.Errorf("failed to delete access key: %w", err)
	}
	return nil
}

func (s *PostgreSQLStorage) UpdateAccessKeyUsage(ctx context.Context, accessKeyID string) error {
	query := fmt.Sprintf("UPDATE %s.access_keys SET last_used_at = NOW() WHERE access_key_id = $1", pq.QuoteIdentifier(s.schema))
	_, err := s.db.ExecContext(ctx, query, accessKeyID)
	if err != nil {
		return fmt.Errorf("failed to update access key usage: %w", err)
	}
	return nil
}
