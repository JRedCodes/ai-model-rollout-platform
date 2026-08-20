import { app } from "./app.js";
import { env } from "./config/env.js";
import { connectRedis } from "./redis/redis.client.js";

async function startServer(): Promise<void> {
  try {
    await connectRedis();

    app.listen(env.PORT, () => {
      console.log(`Edge Evaluator listening on port ${env.PORT}`);
    });
  } catch (error) {
    console.error("Failed to start Edge Evaluator:", error);

    process.exit(1);
  }
}

void startServer();
