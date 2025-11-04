package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"simple-message-queue/src/api"
	"simple-message-queue/src/config"
	"simple-message-queue/src/storage"
	"simple-message-queue/src/storage/postgres"
	"simple-message-queue/src/storage/sqlite"

	"github.com/gin-gonic/gin"

	_ "github.com/joho/godotenv/autoload"
)

func main() {
	cfg := config.Load()

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
		storageInstance, err = postgres.NewPostgreSQLStorage(
			cfg.PostgresURL,
			cfg.PostgresHost,
			cfg.PostgresPort,
			cfg.PostgresUser,
			cfg.PostgresPass,
			cfg.PostgresDB,
			"", // Use default schema
		)
		if err != nil {
			log.Fatalf("Failed to initialize PostgreSQL storage: %v", err)
		}
		log.Printf("Using PostgreSQL storage with database: %s", cfg.PostgresDB)
	default:
		log.Fatalf("Unknown storage adapter: %s. Supported adapters: sqlite, postgres", cfg.StorageType)
	}

	defer storageInstance.Close()

	smqHandler := api.NewSMQHandler(storageInstance, cfg.BaseURL, cfg.AdminUsername, cfg.AdminPassword)

	router := gin.New()
	smqHandler.SetupRoutes(router)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		log.Printf("Starting smq server on port %s", cfg.Port)
		log.Printf("Base URL: %s", cfg.BaseURL)
		log.Printf("Storage adapter: %s", cfg.StorageType)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	go startDLQProcessor(storageInstance)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down server...")

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
		ctx := context.Background()
		expiredMessages, err := storageInstance.GetExpiredMessages(ctx)
		if err != nil {
			log.Printf("Error getting expired messages: %v", err)
			continue
		}

		for _, message := range expiredMessages {
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
