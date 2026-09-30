// Synthetic data for browser acceptance and local design previews only.
import { agent, traces } from "./fixture.mjs";

export function populateWorkspace(state) {
  const now = Date.now();
  const iso = (minutes) => new Date(now - minutes * 60000).toISOString();
  state.agents = [
    { ...agent, display_name: "我的 Codex", trace_count: 4, span_count: 32 },
    { ...agent, agent_id: "claude-fixture", display_name: "Claude · 日常开发", runtime_kind: "claude_code", trace_count: 3, span_count: 19 },
    { ...agent, agent_id: "research-fixture", display_name: "研究助手", runtime_kind: "generic", trace_count: 1, span_count: 8 },
  ];
  const titles = [
    "把 Catena 的首页，变成一个舒服的工作空间",
    "梳理这周的产品思考，保留有用的决定",
    "为什么这次部署变慢了？一起检查请求链路",
    "为新功能补上空状态和错误恢复",
    "整理 Agent 的使用习惯与重复问题",
    "重构记忆页面，让内容更容易阅读",
    "比较几种 Trace 的导出方式",
    "核对改动，再写一份简洁的更新记录",
  ];
  state.traces = titles.map((title, i) => ({ ...traces[i % 2], trace_id: `sample-trace-${i + 1}`,
    agent_id: state.agents[i % 3].agent_id, service_name: state.agents[i % 3].display_name,
    session_id: `sample-session-${i + 1}`, input_preview: title, span_count: 4 + i,
    error_count: i === 2 ? 1 : 0, start_time: iso(i * 76 + 10), end_time: iso(i * 76 + 9) }));
  state.jobComplete = true;
  state.completedJob.candidates[0] = { ...state.completedJob.candidates[0], title: "先确认结果，再开始下一步",
    summary: "一份从重复遗漏中整理出的核对方法，给下一次执行多一点确定性。",
    source_trace_ids: state.traces.slice(0, 2).map((trace) => trace.trace_id),
    content: { path: "agent.md", markdown: "# 先确认结果，再开始下一步\n\n每次执行完一个动作，先检查实际结果，再决定接下来的工作。\n\n## 一个简单的习惯\n\n- 查看命令的退出状态和关键输出。\n- 结果不完整时，明确记录仍然未知的部分。\n- 确认变更符合预期后，再向用户报告。\n\n这是一份待实际验证的候选方法。" } };
  state.completedJob.source_trace_ids = state.completedJob.candidates[0].source_trace_ids;
  state.memories = [
    "Catena 的界面应该精简、优美，让人愿意每天打开。复杂的运行详情可以按需展开，内容本身应该优先。",
    "产品需要连接多个 Agent，把经历、记忆与可复用产出放在同一个空间。",
    "每个结论都要保留它的来源，方便回到当时的对话或 Trace 重新理解。",
    "上线前先在本地验证主要流程，尤其是空状态、错误提示和手机阅读体验。",
    "一份好用的方法应该可以带走，在下一次 Agent 工作时继续使用。",
    "用实际的任务经历理解工作习惯，避免从零散记录中给一个人贴标签。",
  ].map((content, i) => ({ id: String(i + 1), content, created_at: iso(i * 125),
    metadata: { conversation_id: `sample-conversation-${i + 1}`, agent_id: state.agents[i % 3].agent_id } }));
  state.memoryReady = true;
  state.memorySearch = { success: true, query: "产品", search_time_ms: 21,
    facts: [{ ...state.memories[0], score: .93 }],
    conversations: [{ id: "conversation-match", title: "关于工作空间的讨论", content: "把阅读体验放在第一位，先展示内容，再按需查看技术详情。", score: .86, metadata: { conversation_id: "sample-conversation-1" } }],
    topics: [{ id: "topic-match", title: "产品方向", content: "从经历中积累可以带走的方法。", score: .81 }] };
}
