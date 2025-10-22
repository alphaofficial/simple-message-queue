
export default async (): Promise<void> => {
  console.log("Starting teardown process...");

  const containers = (global as any).__CONTAINERS__;

  if (containers) {
    try {
      if (containers.smq) {
        await containers.smq.stop();
      }

      if (containers.postgres) {
        await containers.postgres.stop();
      }

      if (containers.network) {
        await containers.network.stop();
      }
    } catch (error) {
      console.error("❌ Error during container cleanup:", error);
    }
  }

  console.log("Teardown completed");
};