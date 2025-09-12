package main

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

func main() {
	// Create AWS session pointing to our local SQS server with query protocol
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String("http://localhost:8080"),
		Credentials: credentials.NewStaticCredentials("dummy", "dummy", ""),
		// Force the SDK to use query protocol instead of JSON
		DisableRestProtocolURICleaning: aws.Bool(true),
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	// Create SQS service client
	sqsClient := sqs.New(sess)

	// Test 1: Create a queue
	queueName := "test-queue"
	createQueueResult, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
		Attributes: map[string]*string{
			"VisibilityTimeout": aws.String("30"),
			"MaxReceiveCount":   aws.String("3"),
		},
	})
	if err != nil {
		log.Fatalf("Failed to create queue: %v", err)
	}

	queueURL := *createQueueResult.QueueUrl
	fmt.Printf("Created queue: %s\n", queueURL)

	// Test 2: Send a message
	sendResult, err := sqsClient.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String("Hello from AWS SQS client!"),
	})
	if err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}

	fmt.Printf("Sent message: %s\n", *sendResult.MessageId)

	// Test 3: Receive messages
	receiveResult, err := sqsClient.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: aws.Int64(1),
		WaitTimeSeconds:     aws.Int64(1),
	})
	if err != nil {
		log.Fatalf("Failed to receive message: %v", err)
	}

	if len(receiveResult.Messages) > 0 {
		message := receiveResult.Messages[0]
		fmt.Printf("Received message: %s - %s\n", *message.MessageId, *message.Body)

		// Test 4: Delete the message
		_, err = sqsClient.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      aws.String(queueURL),
			ReceiptHandle: message.ReceiptHandle,
		})
		if err != nil {
			log.Fatalf("Failed to delete message: %v", err)
		}

		fmt.Println("Deleted message successfully")
	} else {
		fmt.Println("No messages received")
	}

	// Test 5: List queues
	listResult, err := sqsClient.ListQueues(&sqs.ListQueuesInput{})
	if err != nil {
		log.Fatalf("Failed to list queues: %v", err)
	}

	fmt.Printf("Found %d queues:\n", len(listResult.QueueUrls))
	for _, url := range listResult.QueueUrls {
		fmt.Printf("  - %s\n", *url)
	}

	fmt.Println("All tests completed successfully!")
}
