import "dotenv/config";

export const env = {
  PORT: Number(process.env.PORT ?? 4002),
  NODE_ENV: process.env.NODE_ENV ?? "development",
  MODEL_SERVICE_URL: process.env.MODEL_SERVICE_URL ?? "http://localhost:4001",
};