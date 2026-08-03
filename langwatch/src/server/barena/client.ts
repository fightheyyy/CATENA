import type { z } from "zod";
import { env } from "~/env.mjs";
import { problemSchema } from "./contracts";
import {
  barenaGatewayHeaders,
  signBarenaGatewayRequest,
} from "./gateway-signature";

const REQUEST_TIMEOUT_MS = 10_000;

export class BarenaControlPlaneError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = "BarenaControlPlaneError";
  }
}

type RequestOptions = {
  method?: "GET" | "POST";
  body?: unknown;
  idempotencyKey?: string;
  projectId: string;
  actorId: string;
};

export async function requestBarena<TSchema extends z.ZodTypeAny>(
  path: string,
  schema: TSchema,
  options: RequestOptions,
): Promise<z.infer<TSchema>> {
  const baseURL = env.BARENA_CONTROL_PLANE_URL;
  if (!baseURL) {
    throw new BarenaControlPlaneError(
      "Barena control plane is not configured",
      503,
    );
  }
  const gatewaySecret = env.BARENA_GATEWAY_SECRET;
  if (!gatewaySecret) {
    throw new BarenaControlPlaneError(
      "Barena platform gateway signing is not configured",
      503,
    );
  }
  if (!path.startsWith("/v1/") || path.includes("://")) {
    throw new BarenaControlPlaneError("Invalid control-plane path", 500);
  }

  const method = options.method ?? "GET";
  const body = options.body === undefined ? "" : JSON.stringify(options.body);
  const target = new URL(path, ensureTrailingSlash(baseURL));
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const { bodyHash, signature } = signBarenaGatewayRequest(
    {
      method,
      requestUri: `${target.pathname}${target.search}`,
      projectId: options.projectId,
      actorId: options.actorId,
      timestamp,
      body,
    },
    gatewaySecret,
  );
  const headers = new Headers({
    Accept: "application/json",
    [barenaGatewayHeaders.project]: options.projectId,
    [barenaGatewayHeaders.actor]: options.actorId,
    [barenaGatewayHeaders.timestamp]: timestamp,
    [barenaGatewayHeaders.bodyHash]: bodyHash,
    [barenaGatewayHeaders.signature]: signature,
  });
  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  if (options.idempotencyKey) {
    headers.set("Idempotency-Key", options.idempotencyKey);
  }

  let response: Response;
  try {
    response = await fetch(target, {
      method,
      headers,
      body: options.body === undefined ? undefined : body,
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
  } catch {
    throw new BarenaControlPlaneError(
      "Barena control plane is unavailable",
      503,
    );
  }

  const payload = await readJSON(response);
  if (!response.ok) {
    const problem = problemSchema.safeParse(payload);
    const detail = problem.success ? problem.data.detail : undefined;
    throw new BarenaControlPlaneError(
      detail ?? `Control plane returned HTTP ${response.status}`,
      response.status,
    );
  }

  const parsed = schema.safeParse(payload);
  if (!parsed.success) {
    throw new BarenaControlPlaneError(
      "Barena control plane returned an invalid response",
      502,
    );
  }
  return parsed.data;
}

function ensureTrailingSlash(value: string): string {
  return value.endsWith("/") ? value : `${value}/`;
}

async function readJSON(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text.trim()) return undefined;
  try {
    return JSON.parse(text) as unknown;
  } catch {
    throw new BarenaControlPlaneError(
      "Barena control plane returned a non-JSON response",
      502,
    );
  }
}
