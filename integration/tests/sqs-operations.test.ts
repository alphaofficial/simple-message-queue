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

      await sqsClient.send(new DeleteQueueCommand({
        QueueUrl: createResult.QueueUrl
      }));

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

      const sendResult = await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      expect(sendResult.MessageId).toBeDefined();
      expect(sendResult.MD5OfMessageBody).toBeDefined();

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

      const sendResult = await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody,
        MessageAttributes: messageAttributes
      }));

      expect(sendResult.MessageId).toBeDefined();

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

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1
      }));

      expect(receiveResult.Messages).toHaveLength(1);
      const message = receiveResult.Messages![0];

      await sqsClient.send(new DeleteMessageCommand({
        QueueUrl: queueUrl,
        ReceiptHandle: message.ReceiptHandle!
      }));

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

      await sqsClient.send(new SendMessageCommand({
        QueueUrl: queueUrl,
        MessageBody: messageBody
      }));

      const receiveResult1 = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        VisibilityTimeout: visibilityTimeout
      }));

      expect(receiveResult1.Messages).toHaveLength(1);

      const receiveResult2 = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        WaitTimeSeconds: 1
      }));

      expect(receiveResult2.Messages || []).toHaveLength(0);

      await new Promise(resolve => setTimeout(resolve, (visibilityTimeout + 1) * 1000));

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

      const result = await sqsClient.send(new GetQueueAttributesCommand({
        QueueUrl: queueUrl,
        AttributeNames: ["VisibilityTimeout"]
      }));

      expect(result.Attributes!.VisibilityTimeout).toBe(newVisibilityTimeout);
    });
  });

  describe("Authentication", () => {
    it("should fail with invalid access key", async () => {
      const client = new SQSClient({
        region: "us-east-1",
        endpoint: "http://localhost:9324",
        credentials: {
          accessKeyId: "invalid-key",
          secretAccessKey: "invalid-secret"
        }
      });

      await expect(client.send(new ListQueuesCommand({}))).rejects.toThrow();
    });

    it("should succeed with valid access key", async () => {
      const containers = (global as any).__CONTAINERS__;
      const baseUrl = `http://${containers.smqHost}:${containers.smqPort}`;

      const loginResponse = await fetch(`${baseUrl}/login`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: 'username=test-access-key&password=test-secret-key'
      });

      const setCookieHeader = loginResponse.headers.get('set-cookie');
      expect(setCookieHeader).toContain('sqs_session=authenticated');

      const createKeyResponse = await fetch(`${baseUrl}/api/auth/access-keys`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Cookie': 'sqs_session=authenticated'
        },
        body: JSON.stringify({
          name: 'integration-test-key',
          description: 'Key for integration testing'
        })
      });

      expect(createKeyResponse.ok).toBe(true);
      const keyData = await createKeyResponse.json() as {
        accessKeyId: string;
        secretAccessKey: string;
      };
      expect(keyData.accessKeyId).toBeDefined();
      expect(keyData.secretAccessKey).toBeDefined();

      const client = new SQSClient({
        endpoint: baseUrl,
        credentials: {
          accessKeyId: keyData.accessKeyId,
          secretAccessKey: keyData.secretAccessKey
        }
      });

      const result = await client.send(new ListQueuesCommand({}));
      expect(result.QueueUrls).toBeDefined();
    });
  });
});