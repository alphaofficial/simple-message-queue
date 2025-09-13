package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sqs-bridge/src/api"
	"sqs-bridge/src/config"
	"sqs-bridge/src/storage"
	"sqs-bridge/src/storage/sqlite"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize storage based on adapter type
	var storageInstance storage.Storage
	var err error

	switch cfg.StorageType {
	case "sqlite":
		storageInstance, err = sqlite.NewSQLiteStorage(cfg.SQLiteDBPath)
		if err != nil {
			log.Fatalf("Failed to initialize SQLite storage: %v", err)
		}
		log.Printf("Using SQLite storage with database: %s", cfg.SQLiteDBPath)
	case "postgres":
		log.Fatalf("PostgreSQL storage adapter not yet implemented")
	default:
		log.Fatalf("Unknown storage adapter: %s. Supported adapters: sqlite, postgres", cfg.StorageType)
	}

	defer storageInstance.Close()

	// Initialize SQS handler
	sqsHandler := api.NewSQSHandler(storageInstance, cfg.BaseURL)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.Handle("/", sqsHandler)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting SQS server on port %s", cfg.Port)
		log.Printf("Base URL: %s", cfg.BaseURL)
		log.Printf("Storage adapter: %s", cfg.StorageType)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Start background DLQ processor
	go startDLQProcessor(storageInstance)

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func startDLQProcessor(storageInstance storage.Storage) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Process expired messages and move to DLQ
		ctx := context.Background()
		expiredMessages, err := storageInstance.GetExpiredMessages(ctx)
		if err != nil {
			log.Printf("Error getting expired messages: %v", err)
			continue
		}

		for _, message := range expiredMessages {
			// Get queue to find DLQ name
			queue, err := storageInstance.GetQueue(ctx, message.QueueName)
			if err != nil || queue == nil {
				continue
			}

			if queue.DeadLetterQueueName != "" {
				err := storageInstance.MoveMessageToDLQ(ctx, message, queue.DeadLetterQueueName)
				if err != nil {
					log.Printf("Error moving message to DLQ: %v", err)
				} else {
					log.Printf("Moved message %s to DLQ %s", message.ID, queue.DeadLetterQueueName)
				}
			}
		}
	}
}
