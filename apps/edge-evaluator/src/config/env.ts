import "dotenv/config";

export const env = {
  PORT: Number(process.env.PORT ?? 4002),
  NODE_ENV: process.env.NODE_ENV ?? "development",
  MODEL_SERVICE_URL: process.env.MODEL_SERVICE_URL ?? "http://localhost:4001",
  REDIS_URL:
    process.env.REDIS_URL ?? "redis://localhost:6379",
  FEATURE_FLAG_KEY:
    process.env.FEATURE_FLAG_KEY ??
    "feature-flag:model-routing:development",
  TELEMETRY_STREAM_KEY:
    process.env.TELEMETRY_STREAM_KEY ??
    "telemetry:inference-completed",
  STABLE_MODEL_FALLBACK_ID:
    process.env.STABLE_MODEL_FALLBACK_ID ?? "model-v1",
};