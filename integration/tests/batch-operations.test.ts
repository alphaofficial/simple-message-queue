import {
  SQSClient,
  CreateQueueCommand,
  SendMessageBatchCommand,
  DeleteMessageBatchCommand,
  ChangeMessageVisibilityBatchCommand,
  ReceiveMessageCommand,
  SendMessageCommand
} from "@aws-sdk/client-sqs";
import { createSQSClient, generateTestQueueName, cleanupQueue, purgeQueue } from "../testHelpers";

describe("SQS Batch Operations", () => {
  let sqsClient: SQSClient;
  let queueUrl: string;

  beforeAll(async () => {
    sqsClient = createSQSClient();

    const queueName = generateTestQueueName("batch-operations");
    const createResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: queueName
    }));
    queueUrl = createResult.QueueUrl!;
  });

  afterEach(async () => {
    await purgeQueue(sqsClient, queueUrl);
  });

  afterAll(async () => {
    await cleanupQueue(sqsClient, queueUrl);
  });

  describe("SendMessageBatch", () => {
    it("should send multiple messages in a batch", async () => {
      const entries = [
        {
          Id: "msg1",
          MessageBody: "First batch message"
        },
        {
          Id: "msg2",
          MessageBody: "Second batch message"
        },
        {
          Id: "msg3",
          MessageBody: "Third batch message"
        }
      ];

      const result = await sqsClient.send(new SendMessageBatchCommand({
        QueueUrl: queueUrl,
        Entries: entries
      }));

      expect(result.Successful).toBeDefined();
      expect(result.Successful).toHaveLength(3);
      expect(result.Failed || []).toHaveLength(0);

      for (let i = 0; i < entries.length; i++) {
        const successful = result.Successful!.find(s => s.Id === entries[i].Id);
        expect(successful).toBeDefined();
        expect(successful!.MessageId).toBeDefined();
        expect(successful!.MD5OfMessageBody).toBeDefined();
      }
    });

    it("should send batch with message attributes", async () => {
      const entries: Array<{
        Id: string;
        MessageBody: string;
        MessageAttributes?: Record<string, { DataType: string; StringValue: string }>;
      }> = [
        {
          Id: "msg-attr-1",
          MessageBody: "Message with attributes",
          MessageAttributes: {
            "TestAttribute": {
              DataType: "String",
              StringValue: "TestValue1"
            }
          }
        },
        {
          Id: "msg-attr-2",
          MessageBody: "Another message with attributes",
          MessageAttributes: {
            "TestNumber": {
              DataType: "Number",
              StringValue: "42"
            }
          }
        }
      ];

      const result = await sqsClient.send(new SendMessageBatchCommand({
        QueueUrl: queueUrl,
        Entries: entries
      }));

      expect(result.Successful).toHaveLength(2);
      expect(result.Failed || []).toHaveLength(0);

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 2,
        MessageAttributeNames: ["All"]
      }));

      expect(receiveResult.Messages).toHaveLength(2);

      const message1 = receiveResult.Messages!.find(m => m.Body === "Message with attributes");
      const message2 = receiveResult.Messages!.find(m => m.Body === "Another message with attributes");

      expect(message1?.MessageAttributes?.["TestAttribute"]?.StringValue).toBe("TestValue1");
      expect(message2?.MessageAttributes?.["TestNumber"]?.StringValue).toBe("42");
    });

    it("should handle maximum batch size (10 messages)", async () => {
      const entries = Array.from({ length: 10 }, (_, i) => ({
        Id: `max-batch-${i}`,
        MessageBody: `Message ${i + 1} of 10`
      }));

      const result = await sqsClient.send(new SendMessageBatchCommand({
        QueueUrl: queueUrl,
        Entries: entries
      }));

      expect(result.Successful).toHaveLength(10);
      expect(result.Failed || []).toHaveLength(0);
    });

    it("should handle delay seconds in batch", async () => {
      const entries = [
        {
          Id: "delayed-msg-1",
          MessageBody: "Delayed message 1",
          DelaySeconds: 1
        },
        {
          Id: "delayed-msg-2",
          MessageBody: "Delayed message 2",
          DelaySeconds: 2
        }
      ];

      const sendTime = Date.now();
      const result = await sqsClient.send(new SendMessageBatchCommand({
        QueueUrl: queueUrl,
        Entries: entries
      }));

      expect(result.Successful).toHaveLength(2);

      const immediateReceive = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 1
      }));

      const delayedMessages = (immediateReceive.Messages || []).filter(m =>
        m.Body?.includes("Delayed message")
      );
      expect(delayedMessages).toHaveLength(0);
    });
  });

  describe("DeleteMessageBatch", () => {
    it("should delete multiple messages in a batch", async () => {
      const sendEntries = Array.from({ length: 3 }, (_, i) => ({
        Id: `delete-test-${i}`,
        MessageBody: `Message to delete ${i + 1}`
      }));

      await sqsClient.send(new SendMessageBatchCommand({
        QueueUrl: queueUrl,
        Entries: sendEntries
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 3
      }));

      expect(receiveResult.Messages).toHaveLength(3);

      const deleteEntries = receiveResult.Messages!.map((msg, i) => ({
        Id: `delete-${i}`,
        ReceiptHandle: msg.ReceiptHandle!
      }));

      const deleteResult = await sqsClient.send(new DeleteMessageBatchCommand({
        QueueUrl: queueUrl,
        Entries: deleteEntries
      }));

      expect(deleteResult.Successful).toHaveLength(3);
      expect(deleteResult.Failed || []).toHaveLength(0);

      const verifyReceive = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 1
      }));

      const remainingDeleteMessages = (verifyReceive.Messages || []).filter(m =>
        m.Body?.includes("Message to delete")
      );
      expect(remainingDeleteMessages).toHaveLength(0);
    });

    it("should handle partial failures in delete batch", async () => {
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: "Valid message for delete"
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1
      }));

      expect(receiveResult.Messages).toHaveLength(1);

      const deleteEntries = [
        {
          Id: "valid-delete",
          ReceiptHandle: receiveResult.Messages![0].ReceiptHandle!
        },
        {
          Id: "invalid-delete",
          ReceiptHandle: "invalid-receipt-handle"
        }
      ];

      const deleteResult = await sqsClient.send(new DeleteMessageBatchCommand({
        QueueUrl: queueUrl,
        Entries: deleteEntries
      }));

      expect(deleteResult.Successful).toHaveLength(1);
      expect(deleteResult.Failed).toHaveLength(1);

      expect(deleteResult.Successful![0].Id).toBe("valid-delete");
      expect(deleteResult.Failed![0].Id).toBe("invalid-delete");
    });
  });

  describe("ChangeMessageVisibilityBatch", () => {
    it("should change visibility timeout for multiple messages", async () => {
      const sendEntries = Array.from({ length: 2 }, (_, i) => ({
        Id: `visibility-test-${i}`,
        MessageBody: `Visibility test message ${i + 1}`
      }));

      await sqsClient.send(new SendMessageBatchCommand({
        QueueUrl: queueUrl,
        Entries: sendEntries
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 2,
        VisibilityTimeout: 30 // Default timeout
      }));

      expect(receiveResult.Messages).toHaveLength(2);

      const visibilityEntries = receiveResult.Messages!.map((msg, i) => ({
        Id: `visibility-change-${i}`,
        ReceiptHandle: msg.ReceiptHandle!,
        VisibilityTimeout: 1
      }));

      const visibilityResult = await sqsClient.send(new ChangeMessageVisibilityBatchCommand({
        QueueUrl: queueUrl,
        Entries: visibilityEntries
      }));

      expect(visibilityResult.Successful).toHaveLength(2);
      expect(visibilityResult.Failed || []).toHaveLength(0);

      const immediateReceive = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 1
      }));

      const visibilityMessages = (immediateReceive.Messages || []).filter(m =>
        m.Body?.includes("Visibility test message")
      );
      expect(visibilityMessages).toHaveLength(0);

      await new Promise(resolve => setTimeout(resolve, 2000));

      const delayedReceive = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 10
      }));

      const availableMessages = (delayedReceive.Messages || []).filter(m =>
        m.Body?.includes("Visibility test message")
      );
      expect(availableMessages.length).toBeGreaterThan(0);
    });

    it("should make messages immediately visible with zero timeout", async () => {
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: "Immediate visibility test"
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        VisibilityTimeout: 300 // 5 minutes
      }));

      expect(receiveResult.Messages).toHaveLength(1);

      const visibilityResult = await sqsClient.send(new ChangeMessageVisibilityBatchCommand({
        QueueUrl: queueUrl,
        Entries: [{
          Id: "immediate-visibility",
          ReceiptHandle: receiveResult.Messages![0].ReceiptHandle!,
          VisibilityTimeout: 0
        }]
      }));

      expect(visibilityResult.Successful).toHaveLength(1);

      const immediateReceive = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1
      }));

      expect(immediateReceive.Messages).toHaveLength(1);
      expect(immediateReceive.Messages![0].Body).toBe("Immediate visibility test");
    });
  });

  describe("Batch Error Handling", () => {
    it("should handle empty batch requests", async () => {
      await expect(
        sqsClient.send(new SendMessageBatchCommand({
          QueueUrl: queueUrl,
          Entries: []
        }))
      ).rejects.toThrow();

      await expect(
        sqsClient.send(new DeleteMessageBatchCommand({
          QueueUrl: queueUrl,
          Entries: []
        }))
      ).rejects.toThrow();
    });

    it("should handle too many entries (> 10)", async () => {
      const tooManyEntries = Array.from({ length: 11 }, (_, i) => ({
        Id: `too-many-${i}`,
        MessageBody: `Message ${i + 1}`
      }));

      await expect(
        sqsClient.send(new SendMessageBatchCommand({
          QueueUrl: queueUrl,
          Entries: tooManyEntries
        }))
      ).rejects.toThrow();
    });
  });
});