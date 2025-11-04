import {
  SQSClient,
  CreateQueueCommand,
  SendMessageCommand,
  SendMessageBatchCommand,
  ReceiveMessageCommand,
  GetQueueAttributesCommand
} from "@aws-sdk/client-sqs";
import { createSQSClient, generateFifoTestQueueName, cleanupQueue } from "../testHelpers";

describe("SQS FIFO Queue Operations", () => {
  let sqsClient: SQSClient;
  const createdQueues: string[] = [];

  beforeAll(() => {
    sqsClient = createSQSClient();
  });

  afterAll(async () => {
    for (const queueUrl of createdQueues) {
      await cleanupQueue(sqsClient, queueUrl);
    }
  });

  describe("FIFO Queue Creation", () => {
    it("should create a FIFO queue with .fifo suffix", async () => {
      const queueName = generateFifoTestQueueName("basic-fifo");

      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName,
        Attributes: {
          FifoQueue: "true"
        }
      }));

      expect(createResult.QueueUrl).toBeDefined();
      expect(createResult.QueueUrl).toContain(".fifo");
      createdQueues.push(createResult.QueueUrl!);

      const attrsResult = await sqsClient.send(new GetQueueAttributesCommand({
        QueueUrl: createResult.QueueUrl,
        AttributeNames: ["All"]
      }));

      expect(attrsResult.Attributes?.FifoQueue).toBe("true");
    });

    it("should create FIFO queue with content-based deduplication", async () => {
      const queueName = generateFifoTestQueueName("content-dedup");

      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName,
        Attributes: {
          FifoQueue: "true",
          ContentBasedDeduplication: "true"
        }
      }));

      createdQueues.push(createResult.QueueUrl!);

      const attrsResult = await sqsClient.send(new GetQueueAttributesCommand({
        QueueUrl: createResult.QueueUrl,
        AttributeNames: ["ContentBasedDeduplication"]
      }));

      expect(attrsResult.Attributes?.ContentBasedDeduplication).toBe("true");
    });

    it("should fail to create FIFO queue without .fifo suffix", async () => {
      const queueName = "invalid-fifo-queue"; // Missing .fifo suffix

      await expect(
        sqsClient.send(new CreateQueueCommand({
          QueueName: queueName,
          Attributes: {
            FifoQueue: "true"
          }
        }))
      ).rejects.toThrow();
    });
  });

  describe("FIFO Message Ordering", () => {
    let fifoQueueUrl: string;

    beforeAll(async () => {
      const queueName = generateFifoTestQueueName("message-ordering");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName,
        Attributes: {
          FifoQueue: "true"
        }
      }));
      fifoQueueUrl = createResult.QueueUrl!;
      createdQueues.push(fifoQueueUrl);
    });

    it("should maintain FIFO order within message groups", async () => {
      const messageGroupId = "test-group-1";
      const messages = [
        { body: "First message", order: 1 },
        { body: "Second message", order: 2 },
        { body: "Third message", order: 3 }
      ];

      for (const msg of messages) {
        await sqsClient.send(new SendMessageCommand({
          QueueUrl: fifoQueueUrl,
          MessageBody: msg.body,
          MessageGroupId: messageGroupId,
          MessageDeduplicationId: `dedup-${msg.order}-${Date.now()}`
        }));
      }

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: fifoQueueUrl,
        MaxNumberOfMessages: 3
      }));

      expect(receiveResult.Messages).toHaveLength(3);

      expect(receiveResult.Messages![0].Body).toBe("First message");
      expect(receiveResult.Messages![1].Body).toBe("Second message");
      expect(receiveResult.Messages![2].Body).toBe("Third message");
    });

    it("should process different message groups independently", async () => {
      const group1 = "independent-group-1";
      const group2 = "independent-group-2";

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: fifoQueueUrl,
        MessageBody: "Group1-Message1",
        MessageGroupId: group1,
        MessageDeduplicationId: `g1-m1-${Date.now()}`
      }));

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: fifoQueueUrl,
        MessageBody: "Group2-Message1",
        MessageGroupId: group2,
        MessageDeduplicationId: `g2-m1-${Date.now()}`
      }));

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: fifoQueueUrl,
        MessageBody: "Group1-Message2",
        MessageGroupId: group1,
        MessageDeduplicationId: `g1-m2-${Date.now()}`
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: fifoQueueUrl,
        MaxNumberOfMessages: 3
      }));

      expect(receiveResult.Messages).toHaveLength(3);

      const group1Messages = receiveResult.Messages!.filter(m =>
        m.Body?.includes("Group1")
      );
      expect(group1Messages).toHaveLength(2);
      expect(group1Messages[0].Body).toBe("Group1-Message1");
      expect(group1Messages[1].Body).toBe("Group1-Message2");

      const group2Messages = receiveResult.Messages!.filter(m =>
        m.Body?.includes("Group2")
      );
      expect(group2Messages).toHaveLength(1);
      expect(group2Messages[0].Body).toBe("Group2-Message1");
    });
  });

  describe("FIFO Message Deduplication", () => {
    let fifoQueueUrl: string;

    beforeAll(async () => {
      const queueName = generateFifoTestQueueName("deduplication");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName,
        Attributes: {
          FifoQueue: "true"
        }
      }));
      fifoQueueUrl = createResult.QueueUrl!;
      createdQueues.push(fifoQueueUrl);
    });

    it("should deduplicate messages with same MessageDeduplicationId", async () => {
      const messageGroupId = "dedup-test-group";
      const deduplicationId = `dedup-test-${Date.now()}`;
      const messageBody = "Duplicate test message";

      const sendResult1 = await sqsClient.send(new SendMessageCommand({
        QueueUrl: fifoQueueUrl,
        MessageBody: messageBody,
        MessageGroupId: messageGroupId,
        MessageDeduplicationId: deduplicationId
      }));

      const sendResult2 = await sqsClient.send(new SendMessageCommand({
        QueueUrl: fifoQueueUrl,
        MessageBody: messageBody,
        MessageGroupId: messageGroupId,
        MessageDeduplicationId: deduplicationId
      }));

      expect(sendResult1.MessageId).toBeDefined();
      expect(sendResult2.MessageId).toBeDefined();

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: fifoQueueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 2
      }));

      const duplicateTestMessages = (receiveResult.Messages || []).filter(m =>
        m.Body === messageBody
      );
      expect(duplicateTestMessages).toHaveLength(1);
    });

    it("should allow messages with different MessageDeduplicationId", async () => {
      const messageGroupId = "different-dedup-group";
      const baseTime = Date.now();

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: fifoQueueUrl,
        MessageBody: "Message with dedup 1",
        MessageGroupId: messageGroupId,
        MessageDeduplicationId: `dedup1-${baseTime}`
      }));

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: fifoQueueUrl,
        MessageBody: "Message with dedup 2",
        MessageGroupId: messageGroupId,
        MessageDeduplicationId: `dedup2-${baseTime}`
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: fifoQueueUrl,
        MaxNumberOfMessages: 2
      }));

      expect(receiveResult.Messages).toHaveLength(2);
    });

    it("should require MessageGroupId for FIFO queues", async () => {
      await expect(
        sqsClient.send(new SendMessageCommand({
          QueueUrl: fifoQueueUrl,
          MessageBody: "Message without group ID",
          MessageDeduplicationId: `no-group-${Date.now()}`
        }))
      ).rejects.toThrow();
    });

    it("should require MessageDeduplicationId when ContentBasedDeduplication is false", async () => {
      await expect(
        sqsClient.send(new SendMessageCommand({
          QueueUrl: fifoQueueUrl,
          MessageBody: "Message without dedup ID",
          MessageGroupId: "test-group"
        }))
      ).rejects.toThrow();
    });
  });

  describe("FIFO Content-Based Deduplication", () => {
    let contentBasedQueueUrl: string;

    beforeAll(async () => {
      const queueName = generateFifoTestQueueName("content-based-dedup");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName,
        Attributes: {
          FifoQueue: "true",
          ContentBasedDeduplication: "true"
        }
      }));
      contentBasedQueueUrl = createResult.QueueUrl!;
      createdQueues.push(contentBasedQueueUrl);
    });

    it("should deduplicate based on message content", async () => {
      const messageGroupId = "content-dedup-group";
      const messageBody = "Content-based deduplication test";

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: contentBasedQueueUrl,
        MessageBody: messageBody,
        MessageGroupId: messageGroupId
      }));

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: contentBasedQueueUrl,
        MessageBody: messageBody,
        MessageGroupId: messageGroupId
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: contentBasedQueueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 2
      }));

      const contentDedupMessages = (receiveResult.Messages || []).filter(m =>
        m.Body === messageBody
      );
      expect(contentDedupMessages).toHaveLength(1);
    });

    it("should allow different message content", async () => {
      const messageGroupId = "different-content-group";

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: contentBasedQueueUrl,
        MessageBody: "First unique message",
        MessageGroupId: messageGroupId
      }));

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: contentBasedQueueUrl,
        MessageBody: "Second unique message",
        MessageGroupId: messageGroupId
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: contentBasedQueueUrl,
        MaxNumberOfMessages: 2
      }));

      expect(receiveResult.Messages).toHaveLength(2);
    });
  });

  describe("FIFO Batch Operations", () => {
    let fifoQueueUrl: string;

    beforeAll(async () => {
      const queueName = generateFifoTestQueueName("batch-ops");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName,
        Attributes: {
          FifoQueue: "true"
        }
      }));
      fifoQueueUrl = createResult.QueueUrl!;
      createdQueues.push(fifoQueueUrl);
    });

    it("should send FIFO messages in batch", async () => {
      const messageGroupId = "batch-group";
      const baseTime = Date.now();

      const entries = [
        {
          Id: "batch1",
          MessageBody: "Batch message 1",
          MessageGroupId: messageGroupId,
          MessageDeduplicationId: `batch1-${baseTime}`
        },
        {
          Id: "batch2",
          MessageBody: "Batch message 2",
          MessageGroupId: messageGroupId,
          MessageDeduplicationId: `batch2-${baseTime}`
        },
        {
          Id: "batch3",
          MessageBody: "Batch message 3",
          MessageGroupId: messageGroupId,
          MessageDeduplicationId: `batch3-${baseTime}`
        }
      ];

      const result = await sqsClient.send(new SendMessageBatchCommand({
        QueueUrl: fifoQueueUrl,
        Entries: entries
      }));

      expect(result.Successful).toHaveLength(3);
      expect(result.Failed || []).toHaveLength(0);

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: fifoQueueUrl,
        MaxNumberOfMessages: 3
      }));

      expect(receiveResult.Messages).toHaveLength(3);
      expect(receiveResult.Messages![0].Body).toBe("Batch message 1");
      expect(receiveResult.Messages![1].Body).toBe("Batch message 2");
      expect(receiveResult.Messages![2].Body).toBe("Batch message 3");
    });

    it("should handle batch with multiple message groups", async () => {
      const baseTime = Date.now();

      const entries = [
        {
          Id: "multigroup1",
          MessageBody: "Group A - Message 1",
          MessageGroupId: "group-a",
          MessageDeduplicationId: `ga-m1-${baseTime}`
        },
        {
          Id: "multigroup2",
          MessageBody: "Group B - Message 1",
          MessageGroupId: "group-b",
          MessageDeduplicationId: `gb-m1-${baseTime}`
        },
        {
          Id: "multigroup3",
          MessageBody: "Group A - Message 2",
          MessageGroupId: "group-a",
          MessageDeduplicationId: `ga-m2-${baseTime}`
        }
      ];

      const result = await sqsClient.send(new SendMessageBatchCommand({
        QueueUrl: fifoQueueUrl,
        Entries: entries
      }));

      expect(result.Successful).toHaveLength(3);

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: fifoQueueUrl,
        MaxNumberOfMessages: 3
      }));

      expect(receiveResult.Messages).toHaveLength(3);

      const groupAMessages = receiveResult.Messages!
        .filter(m => m.Body?.includes("Group A"))
        .map(m => m.Body);

      expect(groupAMessages).toEqual([
        "Group A - Message 1",
        "Group A - Message 2"
      ]);
    });
  });
});