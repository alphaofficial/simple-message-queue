import {
  SQSClient,
  CreateQueueCommand,
  DeleteQueueCommand,
  ListQueuesCommand,
  SendMessageCommand,
  ReceiveMessageCommand
} from "@aws-sdk/client-sqs";
import { createSQSClient, generateTestQueueName, cleanupQueue } from "../testHelpers";

describe("DLQ Cascade Deletion", () => {
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

  it("should delete DLQ when main queue is deleted", async () => {
    const dlqName = generateTestQueueName("test-dlq");
    const dlqResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: dlqName
    }));
    createdQueues.push(dlqResult.QueueUrl!);

    const mainQueueName = generateTestQueueName("main-queue");
    const redrivePolicy = {
      deadLetterTargetArn: `arn:aws:sqs:us-east-1:123456789012:${dlqName}`,
      maxReceiveCount: 3
    };

    const mainQueueResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: mainQueueName,
      Attributes: {
        RedrivePolicy: JSON.stringify(redrivePolicy)
      }
    }));
    createdQueues.push(mainQueueResult.QueueUrl!);

    const listResult1 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult1.QueueUrls).toContain(mainQueueResult.QueueUrl);
    expect(listResult1.QueueUrls).toContain(dlqResult.QueueUrl);

    await sqsClient.send(new SendMessageCommand({
      QueueUrl: mainQueueResult.QueueUrl,
      MessageBody: "Test message"
    }));

    await sqsClient.send(new DeleteQueueCommand({
      QueueUrl: mainQueueResult.QueueUrl
    }));

    const listResult2 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult2.QueueUrls || []).not.toContain(mainQueueResult.QueueUrl);
    expect(listResult2.QueueUrls || []).not.toContain(dlqResult.QueueUrl);

    const mainIndex = createdQueues.indexOf(mainQueueResult.QueueUrl!);
    const dlqIndex = createdQueues.indexOf(dlqResult.QueueUrl!);
    if (mainIndex > -1) createdQueues.splice(mainIndex, 1);
    if (dlqIndex > -1) createdQueues.splice(dlqIndex, 1);
  });

  it("should delete main queue normally when no DLQ is configured", async () => {
    const queueName = generateTestQueueName("queue-no-dlq");
    const queueResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: queueName
    }));
    createdQueues.push(queueResult.QueueUrl!);

    const listResult1 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult1.QueueUrls).toContain(queueResult.QueueUrl);

    await sqsClient.send(new DeleteQueueCommand({
      QueueUrl: queueResult.QueueUrl
    }));

    const listResult2 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult2.QueueUrls || []).not.toContain(queueResult.QueueUrl);

    const queueIndex = createdQueues.indexOf(queueResult.QueueUrl!);
    if (queueIndex > -1) createdQueues.splice(queueIndex, 1);
  });

  it("should handle deletion gracefully when DLQ doesn't exist", async () => {
    const mainQueueName = generateTestQueueName("main-queue-broken-dlq");
    const nonExistentDlqName = generateTestQueueName("non-existent-dlq");

    const redrivePolicy = {
      deadLetterTargetArn: `arn:aws:sqs:us-east-1:123456789012:${nonExistentDlqName}`,
      maxReceiveCount: 3
    };

    const mainQueueResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: mainQueueName,
      Attributes: {
        RedrivePolicy: JSON.stringify(redrivePolicy)
      }
    }));
    createdQueues.push(mainQueueResult.QueueUrl!);

    const listResult1 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult1.QueueUrls).toContain(mainQueueResult.QueueUrl);

    await sqsClient.send(new DeleteQueueCommand({
      QueueUrl: mainQueueResult.QueueUrl
    }));

    const listResult2 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult2.QueueUrls || []).not.toContain(mainQueueResult.QueueUrl);

    const queueIndex = createdQueues.indexOf(mainQueueResult.QueueUrl!);
    if (queueIndex > -1) createdQueues.splice(queueIndex, 1);
  });

  it("should delete DLQ with messages when main queue is deleted", async () => {
    const dlqName = generateTestQueueName("dlq-with-messages");
    const dlqResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: dlqName
    }));
    createdQueues.push(dlqResult.QueueUrl!);

    const mainQueueName = generateTestQueueName("main-queue-dlq-messages");
    const redrivePolicy = {
      deadLetterTargetArn: `arn:aws:sqs:us-east-1:123456789012:${dlqName}`,
      maxReceiveCount: 2
    };

    const mainQueueResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: mainQueueName,
      Attributes: {
        RedrivePolicy: JSON.stringify(redrivePolicy)
      }
    }));
    createdQueues.push(mainQueueResult.QueueUrl!);

    await sqsClient.send(new SendMessageCommand({
      QueueUrl: mainQueueResult.QueueUrl,
      MessageBody: "Message that will go to DLQ"
    }));

    for (let i = 0; i < 3; i++) {
      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: mainQueueResult.QueueUrl,
        MaxNumberOfMessages: 1,
        VisibilityTimeout: 1
      }));

      if (receiveResult.Messages && receiveResult.Messages.length > 0) {
        await new Promise(resolve => setTimeout(resolve, 1500));
      }
    }

    const dlqReceiveResult = await sqsClient.send(new ReceiveMessageCommand({
      QueueUrl: dlqResult.QueueUrl,
      MaxNumberOfMessages: 1
    }));
    expect(dlqReceiveResult.Messages).toBeDefined();
    expect(dlqReceiveResult.Messages!.length).toBeGreaterThan(0);

    await sqsClient.send(new DeleteQueueCommand({
      QueueUrl: mainQueueResult.QueueUrl
    }));

    const listResult = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult.QueueUrls || []).not.toContain(mainQueueResult.QueueUrl);
    expect(listResult.QueueUrls || []).not.toContain(dlqResult.QueueUrl);

    const mainIndex = createdQueues.indexOf(mainQueueResult.QueueUrl!);
    const dlqIndex = createdQueues.indexOf(dlqResult.QueueUrl!);
    if (mainIndex > -1) createdQueues.splice(mainIndex, 1);
    if (dlqIndex > -1) createdQueues.splice(dlqIndex, 1);
  });
});