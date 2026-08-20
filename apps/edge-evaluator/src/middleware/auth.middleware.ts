import type { NextFunction, Request, Response } from "express";

import { InvalidAPIKeyError, resolveTenantId } from "../repositories/tenant-auth.repository.js";
import { RedisUnavailableError } from "../repositories/feature-flag.repository.js";

declare global {
  namespace Express {
    interface Request {
      tenantId?: string;
    }
  }
}

const BEARER_PREFIX = "Bearer ";

export async function requireTenantAuth(
  req: Request,
  res: Response,
  next: NextFunction,
): Promise<void> {
  const header = req.header("authorization") ?? "";
  const apiKey = header.startsWith(BEARER_PREFIX) ? header.slice(BEARER_PREFIX.length) : "";

  if (!apiKey) {
    res.status(401).json({ error: "MISSING_BEARER_TOKEN" });
    return;
  }

  try {
    req.tenantId = await resolveTenantId(apiKey);
    next();
  } catch (error) {
    if (error instanceof InvalidAPIKeyError) {
      res.status(401).json({ error: "INVALID_API_KEY" });
      return;
    }
    if (error instanceof RedisUnavailableError) {
      res.status(503).json({ error: "AUTH_UNAVAILABLE" });
      return;
    }
    console.error("auth: unexpected error", error);
    res.status(500).json({ error: "AUTH_FAILED" });
  }
}
