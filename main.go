package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sqs-backend/src/api"
	"sqs-backend/src/storage/sqlite"
)

func main() {
	// Configuration
	port := getEnv("PORT", "8080")
	dbPath := getEnv("DB_PATH", "./sqs.db")
	baseURL := getEnv("BASE_URL", fmt.Sprintf("http://localhost:%s", port))

	// Initialize storage
	storage, err := sqlite.NewSQLiteStorage(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storage.Close()

	// Initialize SQS handler
	sqsHandler := api.NewSQSHandler(storage, baseURL)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.Handle("/", sqsHandler)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting SQS server on port %s", port)
		log.Printf("Base URL: %s", baseURL)
		log.Printf("Database: %s", dbPath)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Start background DLQ processor
	go startDLQProcessor(storage)

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

func startDLQProcessor(storage *sqlite.SQLiteStorage) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Process expired messages and move to DLQ
		ctx := context.Background()
		expiredMessages, err := storage.GetExpiredMessages(ctx)
		if err != nil {
			log.Printf("Error getting expired messages: %v", err)
			continue
		}

		for _, message := range expiredMessages {
			// Get queue to find DLQ name
			queue, err := storage.GetQueue(ctx, message.QueueName)
			if err != nil || queue == nil {
				continue
			}

			if queue.DeadLetterQueueName != "" {
				err := storage.MoveMessageToDLQ(ctx, message, queue.DeadLetterQueueName)
				if err != nil {
					log.Printf("Error moving message to DLQ: %v", err)
				} else {
					log.Printf("Moved message %s to DLQ %s", message.ID, queue.DeadLetterQueueName)
				}
			}
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
