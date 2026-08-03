import { createHash, createHmac, timingSafeEqual } from "node:crypto";

export const barenaGatewayHeaders = {
  project: "X-Barena-Project-ID",
  actor: "X-Barena-Actor-ID",
  timestamp: "X-Barena-Gateway-Timestamp",
  bodyHash: "X-Barena-Gateway-Body-SHA256",
  signature: "X-Barena-Gateway-Signature",
} as const;

export type BarenaGatewaySignatureInput = {
  method: string;
  requestUri: string;
  projectId: string;
  actorId: string;
  timestamp: string;
  body: string | Uint8Array;
};

export function hashBarenaGatewayBody(body: string | Uint8Array): string {
  return createHash("sha256").update(body).digest("hex");
}

export function signBarenaGatewayRequest(
  input: BarenaGatewaySignatureInput,
  secret: string,
): { bodyHash: string; signature: string } {
  const bodyHash = hashBarenaGatewayBody(input.body);
  const canonical = canonicalBarenaGatewayRequest(input, bodyHash);
  return {
    bodyHash,
    signature: createHmac("sha256", secret).update(canonical).digest("hex"),
  };
}

export function verifyBarenaGatewayRequest(
  input: BarenaGatewaySignatureInput,
  authentication: {
    bodyHash: string;
    signature: string;
    secret: string;
  },
): boolean {
  const actualBodyHash = hashBarenaGatewayBody(input.body);
  if (!safeEqualHex(actualBodyHash, authentication.bodyHash)) return false;
  const expected = createHmac("sha256", authentication.secret)
    .update(canonicalBarenaGatewayRequest(input, actualBodyHash))
    .digest("hex");
  return safeEqualHex(expected, authentication.signature);
}

function canonicalBarenaGatewayRequest(
  input: Omit<BarenaGatewaySignatureInput, "body">,
  bodyHash: string,
): string {
  return [
    input.method.toUpperCase(),
    input.requestUri,
    input.projectId,
    input.actorId,
    input.timestamp,
    bodyHash,
  ].join("\n");
}

function safeEqualHex(left: string, right: string): boolean {
  if (!/^[a-f0-9]+$/i.test(left) || !/^[a-f0-9]+$/i.test(right)) return false;
  const a = Buffer.from(left, "hex");
  const b = Buffer.from(right, "hex");
  return a.length === b.length && timingSafeEqual(a, b);
}
