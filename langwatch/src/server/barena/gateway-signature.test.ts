import { describe, expect, it } from "vitest";
import {
  hashBarenaGatewayBody,
  signBarenaGatewayRequest,
  verifyBarenaGatewayRequest,
} from "./gateway-signature";

const secret = "test-only-barena-gateway-secret-32-bytes";
const input = {
  method: "POST",
  requestUri: "/v1/platform/scenario-runs/adopt?dry=false",
  projectId: "project-a",
  actorId: "user-a",
  timestamp: "1785484800",
  body: JSON.stringify({ hello: "world" }),
};

describe("Barena gateway signature", () => {
  it("binds method, URI, tenant, actor, timestamp, and exact body", () => {
    const signed = signBarenaGatewayRequest(input, secret);

    expect(signed.bodyHash).toBe(hashBarenaGatewayBody(input.body));
    expect(verifyBarenaGatewayRequest(input, { ...signed, secret })).toBe(true);
    expect(
      verifyBarenaGatewayRequest(
        { ...input, projectId: "project-b" },
        { ...signed, secret },
      ),
    ).toBe(false);
    expect(
      verifyBarenaGatewayRequest(
        { ...input, body: `${input.body} ` },
        { ...signed, secret },
      ),
    ).toBe(false);
  });
});
