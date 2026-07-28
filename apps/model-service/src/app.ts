import express from "express";
import { inferenceRouter } from "./routes/inference.route.js";

export const app = express();

app.use(express.json());

app.get("/health", (_req, res) => {
  res.status(200).json({ status: "ok" });
});

app.use("/v1/models", inferenceRouter);