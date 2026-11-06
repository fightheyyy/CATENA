import assert from "node:assert/strict";
import { before, after, test } from "node:test";
import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";
import { fixtureServer, fakeToken } from "./fixture.mjs";
import { populateWorkspace } from "./workspace-sample.mjs";

let browser;
before(async () => { browser = await chromium.launch({ headless: true, executablePath: process.env.CATENA_BROWSER_EXECUTABLE || undefined }); });
after(async () => { await browser?.close(); });

async function setup(t, options = {}) {
  const fixture = await fixtureServer();
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, permissions: ["clipboard-read", "clipboard-write"], ...options });
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  t.after(async () => { await context.close(); await fixture.close(); assert.deepEqual(errors, [], "no uncaught browser exceptions"); });
  return { ...fixture, page, context };
}

async function visible(locator) { await locator.waitFor({ state: "visible" }); }
async function noOverflow(page) { assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth), false); }
async function screenshot(page, name) {
  if (!process.env.CATENA_BROWSER_ARTIFACTS) return;
  await fs.mkdir(process.env.CATENA_BROWSER_ARTIFACTS, { recursive: true });
  await page.screenshot({ path: path.join(process.env.CATENA_BROWSER_ARTIFACTS, name + "-viewport.png") });
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: path.join(process.env.CATENA_BROWSER_ARTIFACTS, name + ".png"), fullPage: true });
}

test("Settings and Agent routes avoid unrelated endpoints; failed reads remain recoverable", async (t) => {
  const { page, origin, state } = await setup(t);
  await page.goto(origin + "/settings");
  await visible(page.getByRole("heading", { name: "设置", exact: true }));
  assert.deepEqual(state.requests.map((item) => item.path), ["/v1/auth/session"]);
  state.failAgents = true;
  await page.locator(".sidebar").getByRole("button", { name: "Agent", exact: true }).click();
  await visible(page.getByText("Agent query temporarily failed", { exact: true }));
  await page.getByRole("button", { name: "设置", exact: true }).click();
  await visible(page.getByRole("heading", { name: "设置", exact: true }));
  state.failAgents = false;
  await page.locator(".sidebar").getByRole("button", { name: "Agent", exact: true }).click();
  await visible(page.locator(".agent-trace-list button").nth(1));
  assert.equal(state.requests.some((item) => ["/v1/issues", "/v1/cases", "/v1/runtimes", "/v1/traces"].includes(item.path)), false);
  state.failAgents = true;
  await page.getByRole("button", { name: "刷新", exact: true }).click();
  await visible(page.getByText("Agent query temporarily failed", { exact: true }));
  await visible(page.locator(".agent-detail"));
  state.failAgents = false;
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await page.getByText("Agent query temporarily failed", { exact: true }).waitFor({ state: "hidden" });
});

test("An empty workspace offers an Agent connection and labels its sample counts", async (t) => {
  const { page, origin, state } = await setup(t);
  state.agents = [];
  state.emptyTraces = true;
  state.jobs = false;
  await page.goto(origin + "/");
  await visible(page.getByRole("button", { name: "接入 Agent", exact: true }));
  await visible(page.getByText("统计基于最近 100 条 Trace；错误状态不等同于任务失败。", { exact: true }));
  assert.deepEqual(state.requests.map((item) => item.path).sort(), ["/v1/auth/session", "/v1/agents", "/v1/traces", "/v1/analysis-jobs", "/v1/me/llm-config"].sort());
  assert.deepEqual(await page.locator(".proactive-metrics strong").allTextContents(), ["0", "0", "0"]);
  await screenshot(page, "overview-empty");
  await page.getByRole("button", { name: "接入 Agent", exact: true }).click();
  await visible(page.getByRole("heading", { name: "连接", exact: true }));
});

test("Leaving a slow route cancels its request and permits Settings immediately", async (t) => {
  const { page, origin, state } = await setup(t);
  state.holdAgents = true;
  await page.goto(origin + "/agents");
  await visible(page.locator(".sidebar"));
  await page.getByRole("button", { name: "设置", exact: true }).click();
  await visible(page.getByRole("heading", { name: "设置", exact: true }));
  await new Promise((resolve) => setTimeout(resolve, 100));
  assert.equal(state.heldClosed, true);
});

test("Recent Trace opens the exact record, including reload, history and an out-of-window deep link", async (t) => {
  const { page, origin, state } = await setup(t);
  await page.goto(origin + "/agents");
  await page.locator(".agent-trace-list button").nth(1).click();
  await page.waitForURL(/trace=trace-second/);
  await visible(page.locator("#selected-trace-detail").getByText("trace-second", { exact: true }));
  const beforeRefresh = state.requests.filter((item) => item.path === "/v1/traces/trace-second").length;
  const refreshedDetail = page.waitForResponse((response) => response.url().endsWith("/v1/traces/trace-second"));
  await page.getByRole("button", { name: "刷新", exact: true }).click();
  await refreshedDetail;
  assert.ok(state.requests.filter((item) => item.path === "/v1/traces/trace-second").length > beforeRefresh);
  await page.reload();
  await visible(page.locator("#selected-trace-detail").getByText("trace-second", { exact: true }));
  await page.goBack();
  await visible(page.locator(".agent-detail"));
  state.emptyTraces = true;
  await page.goto(origin + "/traces?agent=agent-fixture&trace=trace-outside-window");
  await visible(page.locator('.trace-detail-identity [title="trace-outside-window"]'));
  await noOverflow(page);
  await screenshot(page, "trace-desktop");
});

test("Trace mobile detail supports direct links, failures, retry and return to the index", async (t) => {
  const { page, origin, state } = await setup(t, { viewport: { width: 390, height: 844 } });
  state.failTrace = true;
  await page.goto(origin + "/traces?agent=agent-fixture&trace=trace-second");
  await visible(page.getByText("Requested Trace was not found", { exact: true }));
  await visible(page.getByRole("button", { name: "返回 Trace 列表" }));
  state.failTrace = false;
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await visible(page.locator("#selected-trace-detail").getByText("trace-second", { exact: true }));
  await noOverflow(page);
  await screenshot(page, "trace-mobile");
  await page.getByRole("button", { name: "返回 Trace 列表" }).click();
  assert.equal(await page.locator(".trace-index").isVisible(), true);
  assert.equal(await page.locator(".trace-detail-shell").isVisible(), false);
});

test("Analysis polling publishes sourced unverified artifacts and cancellation retains acknowledgement", async (t) => {
 const {page,origin,state}=await setup(t);
 await page.goto(origin+"/evolution?job=job-fixture");
 await visible(page.getByRole("heading",{name:"分析任务与产物"}));
 await page.getByRole("button",{name:"Cancel",exact:true}).click();
 await page.getByRole("button",{name:"Cancel",exact:true}).waitFor();
 state.jobComplete=true;
 await visible(page.getByRole("heading",{name:"improvement_candidate · unverified"}));
 await visible(page.getByText('Inspect evidence before acting.',{exact:false}));
 await page.getByText("Provenance / evaluation",{exact:true}).click();
 await visible(page.getByText("No real verifier executed",{exact:false}));
 assert.equal(state.requests.some(r=>r.path==="/v1/analysis-jobs/job-fixture/cancel"),true);
 assert.equal(state.requests.some(r=>r.path.startsWith("/v1/experiments")||r.path.startsWith("/v1/evolution-jobs")),false);
 await page.setViewportSize({width:390,height:844}); await noOverflow(page);
});

test("Agent setup copies a shell-safe secret only to clipboard and recovers first-data checks", async (t) => {
  const { page, origin, state } = await setup(t);
  state.agents = [];
  state.failConnection = true;
  await page.goto(origin + "/api-keys");
  await page.getByRole("textbox", { name: "Agent 名称", exact: true }).fill("O'Brien 本地 Agent");
  await page.getByRole("button", { name: "生成接入密钥", exact: true }).click();
  await visible(page.getByText("检测失败，尚未确认接入状态", { exact: true }));
  const guide = page.locator(".agent-connection-guide");
  await guide.getByRole("combobox").selectOption("powershell");
  assert.ok((await guide.locator("pre").innerText()).includes("<CATENA_API_KEY>"));
  await guide.getByRole("button", { name: "复制完整配置" }).click();
  await visible(guide.getByRole("button", { name: "配置已复制" }));
  const clipboard = await page.evaluate(() => navigator.clipboard.readText());
  assert.ok(clipboard.includes(fakeToken));
  assert.ok(clipboard.includes("O''Brien"));
  assert.equal((await page.content()).includes(fakeToken), false);
  assert.equal(await page.evaluate((key) => JSON.stringify(localStorage).includes(key), fakeToken), false);
  await noOverflow(page);
  await screenshot(page, "setup-desktop");
  state.failConnection = false;
  await guide.getByRole("button", { name: "重新检测" }).click();
  await visible(page.getByText("等待首条 Trace 或对话", { exact: true }));
  state.connected = true;
  await visible(page.getByText("已收到这个 Agent 的数据", { exact: true }));
  await guide.getByRole("button", { name: "查看 Agent", exact: true }).click();
  await page.waitForURL(/agents\?agent=new-agent/);
  await visible(page.locator(".agent-detail h2").getByText("O'Brien 本地 Agent", { exact: true }));
});

test("Connection setup fits 390px in English and dark appearance", async (t) => {
  const { page, origin } = await setup(t, { viewport: { width: 390, height: 844 } });
  await page.addInitScript(() => { localStorage.setItem("catena.locale", "en"); localStorage.setItem("catena.theme", "dark"); });
  await page.goto(origin + "/api-keys");
  await page.getByRole("button", { name: "Setup", exact: true }).click();
  await visible(page.locator(".agent-connection-guide"));
  await noOverflow(page);
  assert.deepEqual(await page.locator(".sidebar nav button").evaluateAll((buttons) => buttons.filter((button) => button.scrollWidth > button.clientWidth).map((button) => button.textContent.trim())), []);
  await screenshot(page, "setup-mobile-dark-en");
});

test("Workspace retains exact recent evidence and analysis links", async (t) => {
  const { page, origin, state } = await setup(t);
  populateWorkspace(state);
  await page.goto(origin);
  const history = page.locator(".proactive-list").filter({ has: page.getByRole("heading", { name: "最近经历", exact: true }) });
  const analyses = page.locator(".proactive-list").filter({ has: page.getByRole("heading", { name: /^分析记录/ }) });
  await visible(history.getByRole("button").first());
  assert.deepEqual(await page.locator(".sidebar nav button").allTextContents(), ["工作台", "经历", "产出"]);
  assert.equal(await history.getByRole("button").count(), 4);
  assert.equal(await history.locator("small").first().innerText(), state.traces[0].input_preview);
  await noOverflow(page);
  await screenshot(page, "overview-desktop");
  await history.getByRole("button").filter({ hasText: "为什么这次部署变慢了" }).click();
  await page.waitForURL(/trace=sample-trace-3/);
  await visible(page.locator('.trace-detail-identity [title="sample-trace-3"]'));
  await page.goBack();
  await analyses.getByRole("button").first().click();
  await page.waitForURL(/job=job-fixture/);
  await visible(page.getByRole("heading",{name:"分析任务与产物"}));
});

test("Workspace retains analyses when history fails and can retry the failed read", async (t) => {
  const { page, origin, state } = await setup(t);
  populateWorkspace(state);
  state.failTraceList = true;
  await page.goto(origin);
  await visible(page.getByRole("alert").filter({ hasText: "Trace query temporarily failed" }));
  await visible(page.locator(".proactive-list").last().getByRole("button").first());
  await screenshot(page, "overview-partial-error");
  state.failTraceList = false;
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await visible(page.locator(".proactive-list").first().getByRole("button").first());
  assert.equal(await page.getByRole("alert").filter({ hasText: "Trace query temporarily failed" }).count(), 0);
});

test("Mobile navigation stays reachable and retired conversation links return home", async (t) => {
  const { page, origin, state } = await setup(t, { viewport: { width: 390, height: 844 } });
  populateWorkspace(state);
  await page.goto(origin);
  await visible(page.locator(".proactive-list").first().getByRole("button").first());
  await noOverflow(page);
  await screenshot(page, "overview-mobile");
  const nav = await page.locator(".sidebar nav").boundingBox();
  assert.ok(nav.y >= 740 && nav.y + nav.height <= 845, "primary navigation remains at the bottom of the viewport");
  await page.locator(".sidebar nav").getByRole("button", { name: "经历", exact: true }).click();
  await visible(page.locator(".trace-index"));
  await page.goto(origin + "/conversations");
  await page.waitForURL(/conversations/);
  await visible(page.locator(".proactive-page"));
  assert.equal(await page.locator('.sidebar nav [aria-current="page"]').innerText(), "工作台");
  await noOverflow(page);
});





// UI protocol fixture only; this synthetic table is not Agent quality evidence.
test("Compare preserves unknown cost and Human Review records an explicit decision", async (t) => {
 const { page, origin, state } = await setup(t);
 state.jobComplete = true;
 const artifact = state.completedJob.artifacts[0];
 artifact.validation_status = "review_required";
 const trial = { valid: true, rewards: { reward: 1 }, input_tokens: 10, output_tokens: 2, seconds: 1.5, cost_usd: null, tool_calls: 5, nonzero_command_exits: null };
 artifact.provenance = {compare: [{case_id: "protocol-fixture", held_out: false, baseline: trial, candidate: trial}]};
 await page.goto(origin + "/evolution?job=job-fixture");
 await visible(page.getByRole("heading", {name: "Baseline / Candidate Compare", exact: true}));
 assert.match(await page.locator("table").innerText(), /1 \/ 1/);
 assert.match(await page.locator("table").innerText(), /\? \/ 5/);
 const form = page.getByRole("form", {name: "Human Review", exact: true});
 await form.getByLabel("Reason", {exact: true}).fill("Reviewed synthetic UI fixture only.");
 await form.getByRole("button", {name: "Record decision"}).click();
 await visible(page.getByRole("heading", {name: "Human Review · reject", exact: true}));
 assert.equal(state.completedJob.reviews.length, 1);
 assert.equal(await page.getByRole("button", {name: "Delete", exact: true}).isDisabled(), true);
 await page.reload();
 await visible(page.getByRole("heading", {name: "Human Review · reject", exact: true}));
 await noOverflow(page);
});
