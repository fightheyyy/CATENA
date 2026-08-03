/** Project-scoped internal OTLP ingestion for Barena-owned Replay spans. */

import { createLogger } from "@langwatch/observability";
import type { Context, Next } from "hono";
import { env } from "~/env.mjs";
import { createServiceApp, internalSecret } from "~/server/api/security";
import { getApp } from "~/server/app-layer/app";
import {
  barenaGatewayHeaders,
  verifyBarenaGatewayRequest,
} from "~/server/barena/gateway-signature";
import { prisma } from "~/server/db";
import { DEFAULT_PII_REDACTION_LEVEL } from "~/server/event-sourcing/pipelines/trace-processing/schemas/commands";
import { parseOtlpTraces, readOtlpBody } from "~/server/otel/parseOtlpBody";

const SIGNATURE_WINDOW_SECONDS = 300;
const logger = createLogger("langwatch:barena-internal-otel");

const secured = createServiceApp({
  basePath: "/api/internal/barena/otel/v1",
  verifySecret: verifyBarenaOtlpSignature,
});

const policy = () =>
  internalSecret(
    "Barena gateway HMAC binds Replay OTLP to an authenticated project context",
  );

secured.access(policy()).post("/traces", async (c) => {
  const projectId = c.req.header(barenaGatewayHeaders.project)?.trim() ?? "";
  const project = await prisma.project.findUnique({
    where: { id: projectId },
    select: { id: true },
  });
  if (!project) {
    return c.json({ error: "Barena project not found" }, { status: 404 });
  }

  const body = await readOtlpBody(c.req.raw);
  const parsed = parseOtlpTraces(body, c.req.header("content-type"));
  if (!parsed.ok) {
    logger.warn({ projectId, error: parsed.error }, "invalid Barena OTLP body");
    return c.json({ error: "Failed to parse traces" }, { status: 400 });
  }

  const result = await getApp().traces.collection.handleOtlpTraceRequest(
    project.id,
    parsed.request,
    DEFAULT_PII_REDACTION_LEVEL,
  );
  return c.json({
    partialSuccess: {
      rejectedSpans: result?.rejectedSpans ?? 0,
      errorMessage: result?.errorMessage ?? "",
    },
  });
});

async function verifyBarenaOtlpSignature(c: Context, next: Next) {
  const secret = process.env.BARENA_GATEWAY_SECRET ?? env.BARENA_GATEWAY_SECRET;
  const projectId = c.req.header(barenaGatewayHeaders.project)?.trim() ?? "";
  const actorId = c.req.header(barenaGatewayHeaders.actor)?.trim() ?? "";
  const timestamp = c.req.header(barenaGatewayHeaders.timestamp)?.trim() ?? "";
  const bodyHash = c.req.header(barenaGatewayHeaders.bodyHash)?.trim() ?? "";
  const signature = c.req.header(barenaGatewayHeaders.signature)?.trim() ?? "";
  if (
    !secret ||
    !projectId ||
    projectId.length > 256 ||
    !actorId ||
    actorId.length > 256 ||
    !timestamp ||
    !bodyHash ||
    !signature
  ) {
    return c.json({ error: "Barena platform signature is required" }, 401);
  }

  const body = new Uint8Array(await c.req.raw.clone().arrayBuffer());
  const url = new URL(c.req.url);
  const verified = verifyBarenaGatewayRequest(
    {
      method: c.req.method,
      requestUri: `${url.pathname}${url.search}`,
      projectId,
      actorId,
      timestamp,
      body,
    },
    { bodyHash, signature, secret },
  );
  const unixSeconds = Number.parseInt(timestamp, 10);
  const now = Math.floor(Date.now() / 1000);
  if (
    !verified ||
    !Number.isFinite(unixSeconds) ||
    Math.abs(now - unixSeconds) > SIGNATURE_WINDOW_SECONDS
  ) {
    return c.json({ error: "Invalid Barena platform signature" }, 401);
  }
  await next();
}

export const app = secured.hono;
