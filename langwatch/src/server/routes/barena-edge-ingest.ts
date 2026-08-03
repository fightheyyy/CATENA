/** Public Barena Run/Event ingress authenticated by the existing project key. */

import { createLogger } from "@langwatch/observability";
import type { Context } from "hono";
import type { ContentfulStatusCode } from "hono/utils/http-status";
import { z } from "zod";
import { apiKeyPermission, createProjectApp } from "~/server/api/security";
import { BarenaControlPlaneError, requestBarena } from "~/server/barena/client";
import { runSchema } from "~/server/barena/contracts";

const logger = createLogger("langwatch:barena-edge-ingest");
const MAX_BODY_BYTES = 2 * 1024 * 1024;
const managed = createProjectApp({ basePath: "/api/barena/v1/ingest" });
const manageEvaluations = apiKeyPermission("evaluations:manage");

managed.access(manageEvaluations).post("/runs", async (c) => {
  const body = await readBoundedJSON(c.req.raw);
  if (!body.ok) return c.json({ detail: body.detail }, body.status);

  try {
    const run = await requestBarena("/v1/ingest/runs", runSchema, {
      method: "POST",
      body: body.value,
      ...projectContext(c),
    });
    return c.json(run, 201);
  } catch (error) {
    return controlPlaneFailure(c, error);
  }
});

managed.access(manageEvaluations).post("/runs/:runId/events", async (c) => {
  const body = await readBoundedJSON(c.req.raw);
  if (!body.ok) return c.json({ detail: body.detail }, body.status);

  try {
    await requestBarena(
      `/v1/ingest/runs/${encodeURIComponent(c.req.param("runId"))}/events`,
      z.undefined(),
      {
        method: "POST",
        body: body.value,
        ...projectContext(c),
      },
    );
    return c.body(null, 204);
  } catch (error) {
    return controlPlaneFailure(c, error);
  }
});

managed.access(manageEvaluations).post("/runs/:runId/finish", async (c) => {
  const body = await readBoundedJSON(c.req.raw);
  if (!body.ok) return c.json({ detail: body.detail }, body.status);

  try {
    const run = await requestBarena(
      `/v1/ingest/runs/${encodeURIComponent(c.req.param("runId"))}/finish`,
      runSchema,
      {
        method: "POST",
        body: body.value,
        ...projectContext(c),
      },
    );
    return c.json(run, 200);
  } catch (error) {
    return controlPlaneFailure(c, error);
  }
});

function projectContext(c: Context) {
  const project = c.get("project") as { id: string };
  const apiKeyUserId = c.get("apiKeyUserId") as string | undefined;
  const apiKeyId = c.get("apiKeyId") as string | undefined;
  return {
    projectId: project.id,
    actorId: apiKeyUserId
      ? `api-key-user:${apiKeyUserId}`
      : apiKeyId
        ? `api-key:${apiKeyId}`
        : "project-api-key",
  };
}

async function readBoundedJSON(
  request: Request,
): Promise<
  | { ok: true; value: unknown }
  | { ok: false; status: 400 | 413; detail: string }
> {
  const declaredLength = Number.parseInt(
    request.headers.get("content-length") ?? "0",
    10,
  );
  if (Number.isFinite(declaredLength) && declaredLength > MAX_BODY_BYTES) {
    return { ok: false, status: 413, detail: "Request body is too large" };
  }

  const bytes = new Uint8Array(await request.arrayBuffer());
  if (bytes.byteLength > MAX_BODY_BYTES) {
    return { ok: false, status: 413, detail: "Request body is too large" };
  }
  if (bytes.byteLength === 0) {
    return { ok: false, status: 400, detail: "JSON body is required" };
  }
  try {
    return {
      ok: true,
      value: JSON.parse(new TextDecoder().decode(bytes)) as unknown,
    };
  } catch {
    return { ok: false, status: 400, detail: "Request body must be JSON" };
  }
}

function controlPlaneFailure(c: Context, error: unknown) {
  if (error instanceof BarenaControlPlaneError) {
    const status = publicStatus(error.status);
    return c.json({ status, detail: error.message }, status);
  }
  logger.error({ error }, "Barena edge ingress proxy failed");
  return c.json(
    {
      status: 502,
      detail: "Barena control plane returned an invalid response",
    },
    502,
  );
}

function publicStatus(status: number): ContentfulStatusCode {
  switch (status) {
    case 400:
    case 401:
    case 403:
    case 404:
    case 409:
    case 413:
    case 429:
    case 500:
    case 502:
    case 503:
      return status;
    default:
      return 502;
  }
}

export const app = managed.hono;
