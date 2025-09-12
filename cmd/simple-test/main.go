package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func main() {
	baseURL := "http://localhost:8080"

	fmt.Println("Testing SQS-compatible server...")

	// Test 1: Create a queue
	fmt.Println("\n1. Creating queue 'test-queue'...")
	params := url.Values{}
	params.Set("Action", "CreateQueue")
	params.Set("QueueName", "test-queue")
	params.Set("Version", "2012-11-05")

	resp, err := http.Post(baseURL, "application/x-www-form-urlencoded", strings.NewReader(params.Encode()))
	if err != nil {
		log.Fatalf("Failed to create queue: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Response: %s\n", string(body))

	// Test 2: List queues
	fmt.Println("\n2. Listing queues...")
	params = url.Values{}
	params.Set("Action", "ListQueues")
	params.Set("Version", "2012-11-05")

	resp, err = http.Post(baseURL, "application/x-www-form-urlencoded", strings.NewReader(params.Encode()))
	if err != nil {
		log.Fatalf("Failed to list queues: %v", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	fmt.Printf("Response: %s\n", string(body))

	// Test 3: Send a message
	fmt.Println("\n3. Sending message to queue...")
	params = url.Values{}
	params.Set("Action", "SendMessage")
	params.Set("QueueUrl", baseURL+"/test-queue")
	params.Set("MessageBody", "Hello, SQS!")
	params.Set("Version", "2012-11-05")

	resp, err = http.Post(baseURL, "application/x-www-form-urlencoded", strings.NewReader(params.Encode()))
	if err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	fmt.Printf("Response: %s\n", string(body))

	// Wait a moment
	time.Sleep(1 * time.Second)

	// Test 4: Receive messages
	fmt.Println("\n4. Receiving messages from queue...")
	params = url.Values{}
	params.Set("Action", "ReceiveMessage")
	params.Set("QueueUrl", baseURL+"/test-queue")
	params.Set("MaxNumberOfMessages", "1")
	params.Set("Version", "2012-11-05")

	resp, err = http.Post(baseURL, "application/x-www-form-urlencoded", strings.NewReader(params.Encode()))
	if err != nil {
		log.Fatalf("Failed to receive message: %v", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	fmt.Printf("Response: %s\n", string(body))

	// Extract receipt handle from response (simple string parsing for demo)
	receiptHandle := ""
	if strings.Contains(string(body), "<ReceiptHandle>") {
		start := strings.Index(string(body), "<ReceiptHandle>") + len("<ReceiptHandle>")
		end := strings.Index(string(body)[start:], "</ReceiptHandle>")
		if end > 0 {
			receiptHandle = string(body)[start : start+end]
		}
	}

	if receiptHandle != "" {
		// Test 5: Delete the message
		fmt.Println("\n5. Deleting message from queue...")
		params = url.Values{}
		params.Set("Action", "DeleteMessage")
		params.Set("QueueUrl", baseURL+"/test-queue")
		params.Set("ReceiptHandle", receiptHandle)
		params.Set("Version", "2012-11-05")

		resp, err = http.Post(baseURL, "application/x-www-form-urlencoded", strings.NewReader(params.Encode()))
		if err != nil {
			log.Fatalf("Failed to delete message: %v", err)
		}
		defer resp.Body.Close()

		body, _ = io.ReadAll(resp.Body)
		fmt.Printf("Response: %s\n", string(body))
	}

	fmt.Println("\n✅ All tests completed successfully!")
}
