import {
  SQSClient,
  CreateQueueCommand,
  DeleteQueueCommand,
  SendMessageCommand,
  SendMessageBatchCommand,
  GetQueueAttributesCommand,
  PurgeQueueCommand
} from "@aws-sdk/client-sqs";
import { Consumer } from "sqs-consumer";
import { createSQSClient, generateTestQueueName, cleanupQueue } from "../testHelpers";

describe("SQS Consumer Integration Tests", () => {
  let sqsClient: SQSClient;
  const createdQueues: string[] = [];

  beforeAll(() => {
    sqsClient = createSQSClient();
  });

  afterAll(async () => {
    // Cleanup all created queues
    for (const queueUrl of createdQueues) {
      await cleanupQueue(sqsClient, queueUrl);
    }
  });

  describe("Basic Message Consumption", () => {
    let queueUrl: string;

    beforeAll(async () => {
      const queueName = generateTestQueueName("consumer-basic");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));
      queueUrl = createResult.QueueUrl!;
      createdQueues.push(queueUrl);
    });

    it("should consume a single message successfully", async () => {
      const messageBody = "Test message for consumer";
      const receivedMessages: string[] = [];

      // Send a test message
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      // Create consumer with timeout to prevent infinite polling
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        waitTimeSeconds: 2,
        pollingWaitTimeMs: 1000,
        handleMessage: async (message) => {
          receivedMessages.push(message.Body!);
        }
      });

      // Start consumer and wait for message processing
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 5000)); // Wait for message processing
      
      consumer.stop();

      expect(receivedMessages).toHaveLength(1);
      expect(receivedMessages[0]).toBe(messageBody);
    });

    it("should consume multiple messages", async () => {
      const messageCount = 5;
      const receivedMessages: string[] = [];

      // Send multiple test messages
      for (let i = 0; i < messageCount; i++) {
        await sqsClient.send(new SendMessageCommand({
          QueueUrl: queueUrl,
          MessageBody: `Test message ${i + 1}`
        }));
      }

      // Create consumer with timeout to prevent infinite polling
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        waitTimeSeconds: 2,
        pollingWaitTimeMs: 1000,
        handleMessage: async (message) => {
          receivedMessages.push(message.Body!);
        }
      });

      // Start consumer and wait for message processing
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 10000)); // Wait for all messages
      
      consumer.stop();

      expect(receivedMessages).toHaveLength(messageCount);
      receivedMessages.forEach((msg, index) => {
        expect(msg).toBe(`Test message ${index + 1}`);
      });
    });

    it("should consume messages with attributes", async () => {
      const messageBody = "Message with attributes";
      const messageAttributes = {
        "CustomAttribute": {
          DataType: "String",
          StringValue: "CustomValue"
        },
        "Priority": {
          DataType: "Number",
          StringValue: "5"
        }
      };

      let receivedMessage: any = null;

      // Send message with attributes
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody,
        MessageAttributes: messageAttributes
      }));

      // Create consumer with promise-based completion
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        waitTimeSeconds: 2,
        pollingWaitTimeMs: 1000,
        messageAttributeNames: ["All"],
        handleMessage: async (message) => {
          receivedMessage = message;
          return Promise.resolve();
        }
      });

      // Start consumer and wait for message processing
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 5000));
      
      consumer.stop();

      expect(receivedMessage).not.toBeNull();
      expect(receivedMessage.Body).toBe(messageBody);
      expect(receivedMessage.MessageAttributes).toBeDefined();
      expect(receivedMessage.MessageAttributes["CustomAttribute"].StringValue).toBe("CustomValue");
      expect(receivedMessage.MessageAttributes["Priority"].StringValue).toBe("5");
    });
  });

  describe("Batch Message Consumption", () => {
    let queueUrl: string;

    beforeAll(async () => {
      const queueName = generateTestQueueName("consumer-batch");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));
      queueUrl = createResult.QueueUrl!;
      createdQueues.push(queueUrl);
    });

    it("should consume messages in batches", async () => {
      const batchSize = 3;
      const totalMessages = 9;
      const receivedMessages: string[] = [];

      // Send messages in batch
      const entries = [];
      for (let i = 0; i < totalMessages; i++) {
        entries.push({
          Id: `msg-${i}`,
          MessageBody: `Batch message ${i + 1}`
        });
      }

      // Send in batches of 10 (SQS limit)
      for (let i = 0; i < entries.length; i += 10) {
        const batch = entries.slice(i, i + 10);
        await sqsClient.send(new SendMessageBatchCommand({
          QueueUrl: queueUrl,
          Entries: batch
        }));
      }

      // Create consumer with batch processing
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        batchSize,
        handleMessage: async (message) => {
          receivedMessages.push(message.Body!);
        }
      });

      // Start consumer and wait for message processing
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 5000));
      
      consumer.stop();

      expect(receivedMessages).toHaveLength(totalMessages);
    });
  });

  describe("Error Handling", () => {
    let queueUrl: string;

    beforeAll(async () => {
      const queueName = generateTestQueueName("consumer-errors");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName,
        Attributes: {
          VisibilityTimeout: "10", // Short timeout for faster error testing
          MessageRetentionPeriod: "300" // 5 minutes for testing
        }
      }));
      queueUrl = createResult.QueueUrl!;
      createdQueues.push(queueUrl);
    });

    it("should handle processing errors gracefully", async () => {
      const messageBody = "Error test message";
      const errors: Error[] = [];
      const processedMessages: string[] = [];

      // Send test message
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      // Create consumer that throws an error
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        waitTimeSeconds: 2,
        pollingWaitTimeMs: 1000,
        handleMessage: async (message) => {
          if (message.Body === messageBody) {
            throw new Error("Simulated processing error");
          }
          processedMessages.push(message.Body!);
        }
      });

      // Listen for errors
      consumer.on("error", (err) => {
        errors.push(err);
      });

      consumer.on("processing_error", (err) => {
        errors.push(err);
      });

      // Start consumer and wait for error
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 5000));
      
      consumer.stop();

      expect(errors.length).toBeGreaterThanOrEqual(1);
      expect(errors[0].message).toContain("Simulated processing error");
      expect(processedMessages).toHaveLength(0);
    });

    it("should retry failed messages based on visibility timeout", async () => {
      const messageBody = "Retry test message";
      let processAttempts = 0;
      const maxAttempts = 2;

      // Send test message
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      // Create consumer that fails first attempt, succeeds second
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        visibilityTimeout: 5, // 5 seconds for faster testing
        handleMessage: async (message) => {
          processAttempts++;
          if (processAttempts < maxAttempts) {
            throw new Error(`Attempt ${processAttempts} failed`);
          }
          // Success on second attempt
          expect(message.Body).toBe(messageBody);
        }
      });

      // Start consumer and wait for retry
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 15000)); // Wait longer for retry
      
      consumer.stop();

      expect(processAttempts).toBe(maxAttempts);
    });
  });

  describe("Consumer Configuration", () => {
    let queueUrl: string;

    beforeAll(async () => {
      const queueName = generateTestQueueName("consumer-config");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));
      queueUrl = createResult.QueueUrl!;
      createdQueues.push(queueUrl);
    });

    it("should respect polling configuration", async () => {
      const messageBody = "Polling config test";
      const receivedMessages: string[] = [];

      // Send test message
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      // Create consumer with specific polling config
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        waitTimeSeconds: 20, // Long polling
        pollingWaitTimeMs: 1000, // 1 second between polls
        handleMessage: async (message) => {
          receivedMessages.push(message.Body!);
        }
      });

      // Start consumer
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 3000));
      
      consumer.stop();

      expect(receivedMessages).toHaveLength(1);
      expect(receivedMessages[0]).toBe(messageBody);
    });

    it("should handle consumer lifecycle events", async () => {
      const events: string[] = [];

      // Create consumer
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        handleMessage: async (message) => {
          // Message handler
        }
      });

      // Listen for lifecycle events
      consumer.on("started", () => events.push("started"));
      consumer.on("stopped", () => events.push("stopped"));
      consumer.on("empty", () => events.push("empty"));

      // Start and stop consumer
      consumer.start();
      await new Promise(resolve => setTimeout(resolve, 1000));
      consumer.stop();
      await new Promise(resolve => setTimeout(resolve, 500));

      expect(events).toContain("started");
      expect(events).toContain("stopped");
      expect(events).toContain("empty"); // Should be empty since no messages
    });
  });

  describe("Message Processing States", () => {
    let queueUrl: string;

    beforeAll(async () => {
      const queueName = generateTestQueueName("consumer-states");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));
      queueUrl = createResult.QueueUrl!;
      createdQueues.push(queueUrl);
    });

    it("should track message processing states", async () => {
      const messageBody = "State tracking test";
      const states: string[] = [];

      // Send test message
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      // Create consumer with state tracking
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        handleMessage: async (message) => {
          states.push("processing");
          await new Promise(resolve => setTimeout(resolve, 100)); // Simulate processing
          states.push("completed");
        }
      });

      // Listen for events
      consumer.on("message_received", () => states.push("received"));
      consumer.on("message_processed", () => states.push("processed"));

      // Start consumer
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 2000));
      
      consumer.stop();

      expect(states).toContain("received");
      expect(states).toContain("processing");
      expect(states).toContain("completed");
      expect(states).toContain("processed");
    });

    it("should handle concurrent message processing", async () => {
      const messageCount = 5;
      const processedMessages: string[] = [];
      const processingTimes: number[] = [];

      // Send multiple messages
      for (let i = 0; i < messageCount; i++) {
        await sqsClient.send(new SendMessageCommand({
          QueueUrl: queueUrl,
          MessageBody: `Concurrent message ${i + 1}`
        }));
      }

      // Create consumer with concurrency
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        batchSize: 2, // Process 2 at a time
        handleMessage: async (message) => {
          const startTime = Date.now();
          await new Promise(resolve => setTimeout(resolve, 500)); // Simulate processing time
          const endTime = Date.now();
          
          processedMessages.push(message.Body!);
          processingTimes.push(endTime - startTime);
        }
      });

      // Start consumer
      consumer.start();
      
      await new Promise(resolve => setTimeout(resolve, 8000)); // Wait for all messages
      
      consumer.stop();

      expect(processedMessages).toHaveLength(messageCount);
      expect(processingTimes.every(time => time >= 450)).toBe(true); // Allow for 50ms variance
    });
  });

  describe("Queue Monitoring", () => {
    let queueUrl: string;

    beforeAll(async () => {
      const queueName = generateTestQueueName("consumer-monitoring");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));
      queueUrl = createResult.QueueUrl!;
      createdQueues.push(queueUrl);
    });

    beforeEach(async () => {
      // Clean queue before each test
      await sqsClient.send(new PurgeQueueCommand({ QueueUrl: queueUrl }));
      await new Promise(resolve => setTimeout(resolve, 1000)); // Wait for purge
    });

    it("should monitor queue attributes during consumption", async () => {
      const messageCount = 3;
      let initialMessageCount = 0;
      let finalMessageCount = 0;

      // Send test messages
      for (let i = 0; i < messageCount; i++) {
        await sqsClient.send(new SendMessageCommand({
          QueueUrl: queueUrl,
          MessageBody: `Monitoring test message ${i + 1}`
        }));
      }

      // Wait for messages to be available
      await new Promise(resolve => setTimeout(resolve, 1000));

      // Check initial queue attributes
      const initialAttrs = await sqsClient.send(new GetQueueAttributesCommand({
        QueueUrl: queueUrl,
        AttributeNames: ["ApproximateNumberOfMessages"]
      }));
      initialMessageCount = parseInt(initialAttrs.Attributes!.ApproximateNumberOfMessages || "0");

      // Create consumer to process all messages
      const consumer = Consumer.create({
        queueUrl,
        sqs: sqsClient,
        handleMessage: async (message) => {
          // Process message
          await new Promise(resolve => setTimeout(resolve, 100));
        }
      });

      // Start consumer and wait for processing
      consumer.start();
      await new Promise(resolve => setTimeout(resolve, 5000));
      consumer.stop();

      // Wait a bit for queue attributes to update
      await new Promise(resolve => setTimeout(resolve, 2000));

      // Check final queue attributes
      const finalAttrs = await sqsClient.send(new GetQueueAttributesCommand({
        QueueUrl: queueUrl,
        AttributeNames: ["ApproximateNumberOfMessages"]
      }));
      finalMessageCount = parseInt(finalAttrs.Attributes!.ApproximateNumberOfMessages || "0");

      expect(initialMessageCount).toBe(messageCount);
      expect(finalMessageCount).toBe(0);
    });
  });
});