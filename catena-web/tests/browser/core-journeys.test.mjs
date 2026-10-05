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
  assert.deepEqual(state.requests.map((item) => item.path).sort(), ["/v1/auth/session", "/v1/agents", "/v1/traces", "/v1/evolution-jobs", "/v1/me/llm-config"].sort());
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

test("A polled analysis publishes its asset into the shared library without reloading", async (t) => {
  const { page, origin, state } = await setup(t);
  await page.goto(origin + "/evolution?job=job-fixture");
  await visible(page.locator(".job-detail-column"));
  state.jobComplete = true;
  await visible(page.getByRole("button", { name: "删除分析", exact: true }));
  await page.getByRole("button", { name: "返回产出", exact: true }).click();
  await visible(page.getByText("先核对再回答", { exact: true }).first());
  assert.ok(state.requests.filter((item) => item.path.endsWith("/job-fixture")).length < 8, "poll results must not retrigger an immediate fetch loop");
  await screenshot(page, "assets-desktop");
});

test("An analysis finishes while the asset library is open, in both narrow and desktop layouts", async (t) => {
  const { page, origin, state } = await setup(t, { viewport: { width: 390, height: 844 } });
  await page.goto(origin + "/evolution");
  await visible(page.locator(".evolution-empty"));
  state.jobComplete = true;
  await visible(page.getByText("先核对再回答", { exact: true }).first());
  await noOverflow(page);
  await screenshot(page, "assets-mobile");
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
  assert.deepEqual(await page.locator(".sidebar nav button").allTextContents(), ["工作台", "经历", "记忆", "实验", "产出"]);
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
  await visible(page.locator(".job-detail-column"));
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

test("Mobile navigation stays reachable and legacy conversation links retain History selection", async (t) => {
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
  await visible(page.locator(".conversation-index"));
  assert.equal(await page.locator('.sidebar nav [aria-current="page"]').innerText(), "经历");
  await noOverflow(page);
});

test("Memory starts as readable cards, retains mixed search results and supports returning to the collection", async (t) => {
  const { page, origin, state } = await setup(t);
  populateWorkspace(state);
  await page.goto(origin + "/memory");
  await visible(page.locator(".memory-result").first());
  assert.equal(await page.locator(".memory-result").count(), 6);
  assert.equal(state.requests.some((request) => request.path.endsWith("/graph")), false);
  assert.equal(await page.locator(".memory-task-disclosure").count(), 0);
  await screenshot(page, "memory-desktop");
  await page.locator(".memory-result").first().click();
  assert.ok((await page.locator(".memory-reader").innerText()).includes(state.memories[0].content));
  assert.ok((await page.locator(".memory-reader").innerText()).includes("sample-conversation-1"));
  await page.getByRole("button", { name: "查看相关记忆" }).click();
  await visible(page.locator(".react-flow"));
  assert.ok(state.requests.some((request) => request.path === "/v1/memories/facts/1/graph"));
  await page.getByRole("button", { name: "卡片", exact: true }).click();
  await page.getByRole("textbox", { name: "找回一段记忆" }).fill("产品");
  await page.locator(".memory-search").getByRole("button").click();
  await visible(page.getByRole("button", { name: "返回全部记忆" }));
  assert.equal(await page.locator(".memory-result").count(), 3, "facts, conversations and topics remain readable");
  await page.locator(".memory-result").nth(1).click();
  await visible(page.locator(".memory-reader h2").getByText("关于工作空间的讨论"));
  await page.getByRole("button", { name: "返回全部记忆" }).click();
  assert.equal(await page.locator(".memory-result").count(), 6);
  assert.equal(await page.getByRole("textbox", { name: "找回一段记忆" }).inputValue(), "");
  await noOverflow(page);
});

test("Memory and workspace fit narrow screens in English and dark appearance", async (t) => {
  const { page, origin, state } = await setup(t, { viewport: { width: 390, height: 844 } });
  populateWorkspace(state);
  await page.addInitScript(() => { localStorage.setItem("catena.locale", "en"); localStorage.setItem("catena.theme", "dark"); });
  await page.goto(origin);
  await visible(page.locator(".proactive-list").first().getByRole("button").first());
  await noOverflow(page);
  await screenshot(page, "overview-mobile-dark-en");
  await page.locator(".sidebar nav").getByRole("button", { name: "Memory", exact: true }).click();
  await visible(page.locator(".memory-result").first());
  await noOverflow(page);
  await screenshot(page, "memory-mobile-dark-en");
  await page.locator(".memory-result").nth(4).click();
  await visible(page.locator(".memory-reader"));
  assert.equal(await page.locator(".memory-reader").evaluate((element) => document.activeElement === element), true);
  const readerBox = await page.locator(".memory-reader").boundingBox();
  assert.ok(readerBox.y >= 64 && readerBox.y < 650, "opening a lower card brings its reader into view below the fixed header");
  await noOverflow(page);
});

test("Memory read failures are recoverable and never described as an empty collection", async (t) => {
  const { page, origin, state } = await setup(t);
  populateWorkspace(state);
  state.failMemories = true;
  await page.goto(origin + "/memory");
  await visible(page.getByText("无法读取记忆", { exact: true }));
  assert.equal(await page.getByText("还没有长期记忆。打开一段对话，选择“提炼为记忆”。", { exact: true }).count(), 0);
  state.failMemories = false;
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await visible(page.locator(".memory-result").first());
  state.failSearch = true;
  await page.getByRole("textbox", { name: "找回一段记忆" }).fill("产品");
  await page.locator(".memory-search").getByRole("button").click();
  await visible(page.getByText("暂时无法召回，请稍后重试或联系管理员检查记忆配置。", { exact: true }));
  assert.equal(await page.locator(".memory-result").count(), 6);
  state.failSearch = false;
  state.memorySearch = { success: true, query: "产品", facts: [], conversations: [], topics: [] };
  await page.locator(".memory-search").getByRole("button").click();
  await visible(page.getByText("没有找到相关记忆。", { exact: true }));
  await page.getByRole("button", { name: "返回全部记忆" }).click();
  assert.equal(await page.locator(".memory-result").count(), 6);
});

test("A search without fact results cannot keep showing a previous memory graph", async (t) => {
  const { page, origin, state } = await setup(t);
  populateWorkspace(state);
  await page.goto(origin + "/memory");
  await visible(page.locator(".memory-result").first());
  await page.getByRole("button", { name: "关系图", exact: true }).click();
  await visible(page.locator(".react-flow"));
  state.memorySearch.facts = [];
  await page.getByRole("textbox", { name: "找回一段记忆" }).fill("产品");
  await page.locator(".memory-search").getByRole("button").click();
  await visible(page.getByRole("button", { name: "返回全部记忆" }));
  assert.equal(await page.locator(".react-flow").count(), 0);
  await page.getByRole("button", { name: "卡片", exact: true }).click();
  assert.equal(await page.locator(".memory-result").count(), 2);
});
