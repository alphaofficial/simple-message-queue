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
    // Cleanup all created queues
    for (const queueUrl of createdQueues) {
      await cleanupQueue(sqsClient, queueUrl);
    }
  });

  it("should delete DLQ when main queue is deleted", async () => {
    // Step 1: Create DLQ first
    const dlqName = generateTestQueueName("test-dlq");
    const dlqResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: dlqName
    }));
    createdQueues.push(dlqResult.QueueUrl!);

    // Step 2: Create main queue with DLQ configuration
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

    // Step 3: Verify both queues exist
    const listResult1 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult1.QueueUrls).toContain(mainQueueResult.QueueUrl);
    expect(listResult1.QueueUrls).toContain(dlqResult.QueueUrl);

    // Step 4: Send a message to main queue to establish the relationship
    await sqsClient.send(new SendMessageCommand({
      QueueUrl: mainQueueResult.QueueUrl,
      MessageBody: "Test message"
    }));

    // Step 5: Delete main queue
    await sqsClient.send(new DeleteQueueCommand({
      QueueUrl: mainQueueResult.QueueUrl
    }));

    // Step 6: Verify both main queue and DLQ are deleted (cascade deletion)
    const listResult2 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult2.QueueUrls || []).not.toContain(mainQueueResult.QueueUrl);
    expect(listResult2.QueueUrls || []).not.toContain(dlqResult.QueueUrl);

    // Remove from cleanup list since they're already deleted
    const mainIndex = createdQueues.indexOf(mainQueueResult.QueueUrl!);
    const dlqIndex = createdQueues.indexOf(dlqResult.QueueUrl!);
    if (mainIndex > -1) createdQueues.splice(mainIndex, 1);
    if (dlqIndex > -1) createdQueues.splice(dlqIndex, 1);
  });

  it("should delete main queue normally when no DLQ is configured", async () => {
    // Step 1: Create main queue without DLQ
    const queueName = generateTestQueueName("queue-no-dlq");
    const queueResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: queueName
    }));
    createdQueues.push(queueResult.QueueUrl!);

    // Step 2: Verify queue exists
    const listResult1 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult1.QueueUrls).toContain(queueResult.QueueUrl);

    // Step 3: Delete queue
    await sqsClient.send(new DeleteQueueCommand({
      QueueUrl: queueResult.QueueUrl
    }));

    // Step 4: Verify queue is deleted
    const listResult2 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult2.QueueUrls || []).not.toContain(queueResult.QueueUrl);

    // Remove from cleanup list since it's already deleted
    const queueIndex = createdQueues.indexOf(queueResult.QueueUrl!);
    if (queueIndex > -1) createdQueues.splice(queueIndex, 1);
  });

  it("should handle deletion gracefully when DLQ doesn't exist", async () => {
    // Step 1: Create main queue with reference to non-existent DLQ
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

    // Step 2: Verify main queue exists
    const listResult1 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult1.QueueUrls).toContain(mainQueueResult.QueueUrl);

    // Step 3: Delete main queue (should succeed even though DLQ doesn't exist)
    await sqsClient.send(new DeleteQueueCommand({
      QueueUrl: mainQueueResult.QueueUrl
    }));

    // Step 4: Verify main queue is deleted
    const listResult2 = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult2.QueueUrls || []).not.toContain(mainQueueResult.QueueUrl);

    // Remove from cleanup list since it's already deleted
    const queueIndex = createdQueues.indexOf(mainQueueResult.QueueUrl!);
    if (queueIndex > -1) createdQueues.splice(queueIndex, 1);
  });

  it("should delete DLQ with messages when main queue is deleted", async () => {
    // Step 1: Create DLQ
    const dlqName = generateTestQueueName("dlq-with-messages");
    const dlqResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: dlqName
    }));
    createdQueues.push(dlqResult.QueueUrl!);

    // Step 2: Create main queue with DLQ configuration
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

    // Step 3: Send a message to main queue
    await sqsClient.send(new SendMessageCommand({
      QueueUrl: mainQueueResult.QueueUrl,
      MessageBody: "Message that will go to DLQ"
    }));

    // Step 4: Receive and put back the message multiple times to trigger DLQ move
    for (let i = 0; i < 3; i++) {
      const receiveResult = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: mainQueueResult.QueueUrl,
        MaxNumberOfMessages: 1,
        VisibilityTimeout: 1
      }));
      
      if (receiveResult.Messages && receiveResult.Messages.length > 0) {
        // Let visibility timeout expire so message goes back to queue
        await new Promise(resolve => setTimeout(resolve, 1500));
      }
    }

    // Step 5: Verify message moved to DLQ
    const dlqReceiveResult = await sqsClient.send(new ReceiveMessageCommand({
      QueueUrl: dlqResult.QueueUrl,
      MaxNumberOfMessages: 1
    }));
    expect(dlqReceiveResult.Messages).toBeDefined();
    expect(dlqReceiveResult.Messages!.length).toBeGreaterThan(0);

    // Step 6: Delete main queue
    await sqsClient.send(new DeleteQueueCommand({
      QueueUrl: mainQueueResult.QueueUrl
    }));

    // Step 7: Verify both queues are deleted (including DLQ with messages)
    const listResult = await sqsClient.send(new ListQueuesCommand({}));
    expect(listResult.QueueUrls || []).not.toContain(mainQueueResult.QueueUrl);
    expect(listResult.QueueUrls || []).not.toContain(dlqResult.QueueUrl);

    // Remove from cleanup list since they're already deleted
    const mainIndex = createdQueues.indexOf(mainQueueResult.QueueUrl!);
    const dlqIndex = createdQueues.indexOf(dlqResult.QueueUrl!);
    if (mainIndex > -1) createdQueues.splice(mainIndex, 1);
    if (dlqIndex > -1) createdQueues.splice(dlqIndex, 1);
  });
});