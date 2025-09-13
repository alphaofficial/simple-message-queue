import {
  SQSClient,
  CreateQueueCommand,
  DeleteQueueCommand,
  SendMessageCommand,
  ReceiveMessageCommand,
  DeleteMessageCommand,
  ListQueuesCommand,
  GetQueueAttributesCommand,
  SetQueueAttributesCommand
} from "@aws-sdk/client-sqs";
import { createSQSClient, generateTestQueueName, cleanupQueue } from "../testHelpers";

describe("SQS Core Operations", () => {
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

  describe("Queue Management", () => {
    it("should create a standard queue", async () => {
      const queueName = generateTestQueueName("create-standard");

      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));

      expect(createResult.QueueUrl).toBeDefined();
      expect(createResult.QueueUrl).toContain(queueName);

      createdQueues.push(createResult.QueueUrl!);
    });

    it("should create a queue with attributes", async () => {
      const queueName = generateTestQueueName("create-with-attributes");

      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName,
        Attributes: {
          VisibilityTimeout: "60",
          MessageRetentionPeriod: "604800" // 7 days
        }
      }));

      expect(createResult.QueueUrl).toBeDefined();
      createdQueues.push(createResult.QueueUrl!);

      // Verify attributes
      const getAttrsResult = await sqsClient.send(new GetQueueAttributesCommand({
        QueueUrl: createResult.QueueUrl,
        AttributeNames: ["All"]
      }));

      expect(getAttrsResult.Attributes?.VisibilityTimeout).toBe("60");
      expect(getAttrsResult.Attributes?.MessageRetentionPeriod).toBe("604800");
    });

    it("should list queues", async () => {
      const queueName = generateTestQueueName("list-test");

      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));
      createdQueues.push(createResult.QueueUrl!);

      const listResult = await sqsClient.send(new ListQueuesCommand({}));

      expect(listResult.QueueUrls).toBeDefined();
      expect(listResult.QueueUrls).toContain(createResult.QueueUrl);
    });

    it("should delete a queue", async () => {
      const queueName = generateTestQueueName("delete-test");

      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));

      // Delete the queue
      await sqsClient.send(new DeleteQueueCommand({
        QueueUrl: createResult.QueueUrl
      }));

      // Verify it's no longer in the list
      const listResult = await sqsClient.send(new ListQueuesCommand({}));
      expect(listResult.QueueUrls || []).not.toContain(createResult.QueueUrl);
    });
  });

  describe("Message Operations", () => {
    let queueUrl: string;

    beforeAll(async () => {
      const queueName = generateTestQueueName("message-ops");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));
      queueUrl = createResult.QueueUrl!;
      createdQueues.push(queueUrl);
    });

    it("should send and receive a message", async () => {
      const messageBody = "Test message from integration test";

      // Send message
      const sendResult = await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      expect(sendResult.MessageId).toBeDefined();
      expect(sendResult.MD5OfMessageBody).toBeDefined();

      // Receive message
      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1
      }));

      expect(receiveResult.Messages).toBeDefined();
      expect(receiveResult.Messages).toHaveLength(1);

      const receivedMessage = receiveResult.Messages![0];
      expect(receivedMessage.Body).toBe(messageBody);
      expect(receivedMessage.MessageId).toBe(sendResult.MessageId);
      expect(receivedMessage.ReceiptHandle).toBeDefined();
    });

    it("should send message with attributes", async () => {
      const messageBody = "Message with attributes";
      const messageAttributes = {
        "TestAttribute": {
          DataType: "String",
          StringValue: "TestValue"
        },
        "NumberAttribute": {
          DataType: "Number",
          StringValue: "123"
        }
      };

      // Send message with attributes
      const sendResult = await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody,
        MessageAttributes: messageAttributes
      }));

      expect(sendResult.MessageId).toBeDefined();

      // Receive message with attributes
      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        MessageAttributeNames: ["All"]
      }));

      expect(receiveResult.Messages).toHaveLength(1);
      const receivedMessage = receiveResult.Messages![0];

      expect(receivedMessage.MessageAttributes).toBeDefined();
      expect(receivedMessage.MessageAttributes!["TestAttribute"].StringValue).toBe("TestValue");
      expect(receivedMessage.MessageAttributes!["NumberAttribute"].StringValue).toBe("123");
    });

    it("should delete a message", async () => {
      const messageBody = "Message to be deleted";

      // Send message
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      // Receive message
      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1
      }));

      expect(receiveResult.Messages).toHaveLength(1);
      const message = receiveResult.Messages![0];

      // Delete message
      await sqsClient.send(new DeleteMessageCommand({
        QueueUrl: queueUrl,
        ReceiptHandle: message.ReceiptHandle!
      }));

      // Try to receive again - should be empty
      const receiveResult2 = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        WaitTimeSeconds: 1
      }));

      expect(receiveResult2.Messages || []).toHaveLength(0);
    });

    it("should handle visibility timeout", async () => {
      const messageBody = "Visibility timeout test";
      const visibilityTimeout = 2; // 2 seconds

      // Send message
      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      // Receive message with short visibility timeout
      const receiveResult1 = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        VisibilityTimeout: visibilityTimeout
      }));

      expect(receiveResult1.Messages).toHaveLength(1);

      // Try to receive immediately - should be empty (message not visible)
      const receiveResult2 = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        WaitTimeSeconds: 1
      }));

      expect(receiveResult2.Messages || []).toHaveLength(0);

      // Wait for visibility timeout to expire
      await new Promise(resolve => setTimeout(resolve, (visibilityTimeout + 1) * 1000));

      // Try to receive again - should get the message back
      const receiveResult3 = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1
      }));

      expect(receiveResult3.Messages).toHaveLength(1);
      expect(receiveResult3.Messages![0].Body).toBe(messageBody);
    });
  });

  describe("Queue Attributes", () => {
    let queueUrl: string;

    beforeAll(async () => {
      const queueName = generateTestQueueName("queue-attributes");
      const createResult = await sqsClient.send(new CreateQueueCommand({
        QueueName: queueName
      }));
      queueUrl = createResult.QueueUrl!;
      createdQueues.push(queueUrl);
    });

    it("should get queue attributes", async () => {
      const result = await sqsClient.send(new GetQueueAttributesCommand({
        QueueUrl: queueUrl,
        AttributeNames: ["All"]
      }));

      expect(result.Attributes).toBeDefined();
      expect(result.Attributes!.QueueArn).toBeDefined();
      expect(result.Attributes!.ApproximateNumberOfMessages).toBeDefined();
      expect(result.Attributes!.VisibilityTimeout).toBeDefined();
    });

    it("should set queue attributes", async () => {
      const newVisibilityTimeout = "120";

      await sqsClient.send(new SetQueueAttributesCommand({
        QueueUrl: queueUrl,
        Attributes: {
          VisibilityTimeout: newVisibilityTimeout
        }
      }));

      // Verify the attribute was set
      const result = await sqsClient.send(new GetQueueAttributesCommand({
        QueueUrl: queueUrl,
        AttributeNames: ["VisibilityTimeout"]
      }));

      expect(result.Attributes!.VisibilityTimeout).toBe(newVisibilityTimeout);
    });
  });
});