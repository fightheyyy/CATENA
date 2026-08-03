import type { Organization, Project, Team } from "@prisma/client";
import { nanoid } from "nanoid";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { projectFactory } from "~/factories/project.factory";
import { barenaGatewayHeaders } from "~/server/barena/gateway-signature";
import { prisma } from "~/server/db";
import { app } from "../barena-edge-ingest";

const controlURL = "http://barena-control.test";

vi.mock("~/env.mjs", async (importOriginal) => {
  const actual = await importOriginal<typeof import("~/env.mjs")>();
  return {
    env: {
      ...actual.env,
      BARENA_CONTROL_PLANE_URL: "http://barena-control.test",
      BARENA_GATEWAY_SECRET: "test-only-barena-gateway-secret-32-bytes",
    },
  };
});

describe("Barena project API key ingress", () => {
  let organization: Organization;
  let team: Team;
  let project: Project;
  let controlFetch: ReturnType<typeof vi.fn>;
  let controlRequests: Array<{
    url: URL;
    headers: Headers;
    body: Record<string, unknown>;
  }>;

  beforeEach(async () => {
    organization = await prisma.organization.create({
      data: { name: "Spiral Auth Test", slug: `spiral-auth-${nanoid()}` },
    });
    team = await prisma.team.create({
      data: {
        name: "Spiral Auth Team",
        slug: `spiral-auth-team-${nanoid()}`,
        organizationId: organization.id,
      },
    });
    project = await prisma.project.create({
      data: {
        ...projectFactory.build({ slug: `spiral-auth-project-${nanoid()}` }),
        teamId: team.id,
        personalFeatures: {},
      },
    });

    controlRequests = [];
    controlFetch = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = captureControlRequest(input, init);
        controlRequests.push(request);
        return fakeControlResponse(request);
      },
    );
    vi.stubGlobal("fetch", controlFetch);
  });

  afterEach(async () => {
    vi.unstubAllGlobals();
    if (project) await prisma.project.delete({ where: { id: project.id } });
    if (team) await prisma.team.delete({ where: { id: team.id } });
    if (organization) {
      await prisma.organization.delete({ where: { id: organization.id } });
    }
  });

  const request = (path: string, body: unknown, apiKey = project.apiKey) =>
    app.request(path, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${apiKey}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });

  it("rejects a missing or invalid key before contacting Go", async () => {
    const missing = await app.request("/api/barena/v1/ingest/runs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ operation: "explore", input: {} }),
    });
    expect(missing.status).toBe(401);

    const invalid = await request(
      "/api/barena/v1/ingest/runs",
      { operation: "explore", input: {} },
      "sk-lw-not-a-real-key",
    );
    expect(invalid.status).toBe(401);
    expect(controlFetch).not.toHaveBeenCalled();
  });

  it("uses one project key for a complete signed edge Run lifecycle", async () => {
    const created = await request("/api/barena/v1/ingest/runs", {
      operation: "explore",
      input: { scenario: { scenario_id: "project-key" } },
    });
    expect(created.status).toBe(201);

    const event = await request(
      "/api/barena/v1/ingest/runs/run-edge-project/events",
      {
        schema: "barena.engine_event.v1",
        event_id: "run-edge-project.1",
        run_id: "run-edge-project",
        sequence: 1,
        timestamp: "2026-08-02T00:00:00.000Z",
        operation: "explore",
        kind: "terminal",
        phase: "complete",
        actor: "runner",
        payload: { status: "complete" },
      },
    );
    expect(event.status).toBe(204);

    const finished = await request(
      "/api/barena/v1/ingest/runs/run-edge-project/finish",
      { state: "completed" },
    );
    expect(finished.status).toBe(200);
    expect(controlFetch).toHaveBeenCalledTimes(3);
    for (const forwarded of controlRequests) {
      expect(forwarded.url.origin).toBe(controlURL);
      expect(forwarded.headers.get(barenaGatewayHeaders.project)).toBe(
        project.id,
      );
      expect(forwarded.headers.get(barenaGatewayHeaders.actor)).toBe(
        "project-api-key",
      );
      expect(forwarded.headers.get(barenaGatewayHeaders.signature)).toMatch(
        /^[a-f0-9]{64}$/,
      );
      expect(forwarded.headers.has("authorization")).toBe(false);
    }
  });
});

function captureControlRequest(input: RequestInfo | URL, init?: RequestInit) {
  return {
    url: new URL(String(input)),
    headers: new Headers(init?.headers),
    body: init?.body
      ? (JSON.parse(String(init.body)) as Record<string, unknown>)
      : {},
  };
}

function fakeControlResponse(request: {
  url: URL;
  body: Record<string, unknown>;
}) {
  if (request.url.pathname.endsWith("/events")) {
    return new Response(null, { status: 204 });
  }
  const completed = request.url.pathname.endsWith("/finish");
  return new Response(
    JSON.stringify({
      run_id: "run-edge-project",
      request_id: "request-edge-project",
      origin: "edge",
      operation: "explore",
      state: completed ? "completed" : "running",
      current_phase: completed ? "complete" : "starting",
      current_actor: "runner",
      input: request.body.input ?? {},
      cancel_requested: false,
      created_at: "2026-08-02T00:00:00.000Z",
      updated_at: "2026-08-02T00:00:01.000Z",
    }),
    { status: completed ? 200 : 201 },
  );
}
