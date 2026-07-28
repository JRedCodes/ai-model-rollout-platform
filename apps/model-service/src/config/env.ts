import "dotenv/config";

export const env = {
  PORT: Number(process.env.PORT ?? 4001),
  NODE_ENV: process.env.NODE_ENV ?? "development",
};