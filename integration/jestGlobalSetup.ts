import { PostgreSqlContainer } from "@testcontainers/postgresql";
import { GenericContainer, Wait, Network } from "testcontainers";

export default async () => {
  console.log("Starting integration test containers...");

  try {
    // Create a shared network
    const network = await new Network().start();

    // 1. Start PostgreSQL
    console.log("Starting PostgreSQL container...");
    const pgContainer = await new PostgreSqlContainer("postgres:17")
      .withDatabase("simplemessagequeue_test")
      .withUsername("test_user")
      .withPassword("test_password")
      .withNetwork(network)
      .withNetworkAliases("postgres")
      .start();

    console.log(`PostgreSQL started on port ${pgContainer.getMappedPort(5432)}`);

    // 2. Build Simple Message Queue from parent directory
    console.log("Building Simple Message Queue container...");
    const smq = await GenericContainer
      .fromDockerfile("../", "Dockerfile") // Build from parent Go project
      .build();

    // 3. Start Simple Message Queue with PostgreSQL connection
    console.log("Starting Simple Message Queue container...");

    // Use internal network connection
    const pgConnectionUrl = `postgres://test_user:test_password@postgres:5432/simplemessagequeue_test?sslmode=disable`;
    console.log("Database URL:", pgConnectionUrl);

    const smqInstance = await smq
      .withEnvironment({
        STORAGE_ADAPTER: "postgres",
        DATABASE_URL: pgConnectionUrl,
        PORT: "8080",
        ADMIN_USERNAME: "test-access-key",
        ADMIN_PASSWORD: "test-secret-key"
      })
      .withExposedPorts(8080)
      .withNetwork(network)
      .withWaitStrategy(Wait.forListeningPorts())
      .start();

    // Wait a moment for the service to fully start
    await new Promise(resolve => setTimeout(resolve, 3000));

    const smqPort = smqInstance.getMappedPort(8080);
    console.log(`Simple Message Queue started on port ${smqPort}`);

    // Store for global access
    (global as any).__CONTAINERS__ = {
      postgres: pgContainer,
      smq: smqInstance,
      smqPort: smqPort,
      smqHost: smqInstance.getHost(),
      network: network
    };

    console.log("All containers ready for testing!");

  } catch (error) {
    console.error("Failed to start containers:", error);
    throw error;
  }
};