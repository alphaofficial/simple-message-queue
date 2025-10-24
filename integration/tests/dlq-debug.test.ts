import {
  SQSClient,
  CreateQueueCommand,
  GetQueueAttributesCommand,
  ListQueuesCommand
} from "@aws-sdk/client-sqs";
import { createSQSClient, generateTestQueueName, cleanupQueue } from "../testHelpers";

describe("DLQ Debug Tests", () => {
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

  it("should verify DLQ configuration is stored correctly", async () => {
    // Step 1: Create DLQ
    const dlqName = generateTestQueueName("debug-dlq");
    const dlqResult = await sqsClient.send(new CreateQueueCommand({
      QueueName: dlqName
    }));
    createdQueues.push(dlqResult.QueueUrl!);
    console.log("Created DLQ:", dlqResult.QueueUrl);

    // Step 2: Create main queue with DLQ configuration
    const mainQueueName = generateTestQueueName("debug-main");
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
    console.log("Created main queue:", mainQueueResult.QueueUrl);

    // Step 3: Get and verify queue attributes
    const mainAttrs = await sqsClient.send(new GetQueueAttributesCommand({
      QueueUrl: mainQueueResult.QueueUrl,
      AttributeNames: ["All"]
    }));
    
    console.log("Main queue attributes:", JSON.stringify(mainAttrs.Attributes, null, 2));
    
    const dlqAttrs = await sqsClient.send(new GetQueueAttributesCommand({
      QueueUrl: dlqResult.QueueUrl,
      AttributeNames: ["All"]
    }));
    
    console.log("DLQ attributes:", JSON.stringify(dlqAttrs.Attributes, null, 2));

    // Step 4: List all queues
    const listResult = await sqsClient.send(new ListQueuesCommand({}));
    console.log("All queues:", listResult.QueueUrls);

    // Verify RedrivePolicy is set correctly
    expect(mainAttrs.Attributes?.RedrivePolicy).toBeDefined();
    const redriveData = JSON.parse(mainAttrs.Attributes!.RedrivePolicy!);
    expect(redriveData.maxReceiveCount).toBe(3);
    expect(redriveData.deadLetterTargetArn).toContain(dlqName);
  });
});