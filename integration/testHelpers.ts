import { SQSClient } from "@aws-sdk/client-sqs";

interface TestContainers {
  postgres: any;
  smq: any;
  smqPort: number;
  smqHost: string;
}

declare global {
  var __CONTAINERS__: TestContainers;
}

export function createSQSClient(): SQSClient {
  const containers = global.__CONTAINERS__;
  
  if (!containers || !containers.smqPort) {
    throw new Error("Test containers not initialized. Make sure jestGlobalSetup ran successfully.");
  }

  return new SQSClient({
    endpoint: `http://${containers.smqHost}:${containers.smqPort}`,
    region: "us-east-1",
    credentials: {
      accessKeyId: "test-access-key",
      secretAccessKey: "test-secret-key"
    }
  });
}

export function getQueueUrl(queueName: string): string {
  const containers = global.__CONTAINERS__;
  return `http://${containers.smqHost}:${containers.smqPort}/${queueName}`;
}

export function generateTestQueueName(testName: string): string {
  return `test-${testName.replace(/[^a-zA-Z0-9-_]/g, "-")}-${Date.now()}`;
}

export function generateFifoTestQueueName(testName: string): string {
  return `${generateTestQueueName(testName)}.fifo`;
}

export async function cleanupQueue(sqsClient: SQSClient, queueUrl: string): Promise<void> {
  try {
    const { DeleteQueueCommand } = await import("@aws-sdk/client-sqs");
    await sqsClient.send(new DeleteQueueCommand({ QueueUrl: queueUrl }));
  } catch (error) {
    // Ignore cleanup errors
    console.warn(`Failed to cleanup queue ${queueUrl}:`, error);
  }
}

export async function purgeQueue(sqsClient: SQSClient, queueUrl: string): Promise<void> {
  try {
    const { ReceiveMessageCommand, DeleteMessageCommand } = await import("@aws-sdk/client-sqs");
    
    // Keep receiving and deleting messages until queue is empty
    let messagesDeleted = 0;
    while (messagesDeleted < 100) { // Safety limit to prevent infinite loop
      const result = await sqsClient.send(new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 10,
        VisibilityTimeout: 1,
        WaitTimeSeconds: 1
      }));
      
      if (!result.Messages || result.Messages.length === 0) {
        break;
      }
      
      // Delete all received messages
      for (const message of result.Messages) {
        if (message.ReceiptHandle) {
          await sqsClient.send(new DeleteMessageCommand({
            QueueUrl: queueUrl,
            ReceiptHandle: message.ReceiptHandle
          }));
          messagesDeleted++;
        }
      }
    }
  } catch (error) {
    // Ignore purge errors
    console.warn(`Failed to purge queue ${queueUrl}:`, error);
  }
}

export function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}