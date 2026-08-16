import express from "express";
import { inferenceRouter } from "./routes/inference.route.js";
import { requireTenantAuth } from "./middleware/auth.middleware.js";

export const app = express();

app.use(express.json());

app.get("/health", (_req, res) => {
  res.status(200).json({ status: "ok" });
});

app.use("/v1", requireTenantAuth, inferenceRouter);