import { useCallback } from "react";
import { usePublicEnv } from "~/hooks/usePublicEnv";
import { useBarenaI18n } from "./i18n";

const legacyMessages = {
  never: "Never",
  secondsAgo: "{count}s ago",
  minutesAgo: "{count} min ago",
  hoursAgo: "{count}h ago",
  daysAgo: "{count}d ago",
  issuedPersonalKey: "Issued personal key '{label}'",
  issuePersonalKeyError: "Couldn't issue the personal key",
  advancedFeaturesEnabled: "Advanced features enabled",
  advancedFeaturesEnabledDescription:
    "Evaluations, datasets, annotations, and automations now appear in your sidebar.",
  enableAdvancedFeaturesError: "Couldn't enable advanced features",
  advancedFeaturesDisabled: "Advanced features disabled",
  advancedFeaturesDisabledDescription:
    "Sidebar entries hidden. Existing data is preserved and reappears on re-enable.",
  disableAdvancedFeaturesError: "Couldn't disable advanced features",
  keyRevoked: "Key revoked",
  keyRevokedDescription: "The CLI/tool using this key will fail immediately.",
  revokeKeyError: "Couldn't revoke the key",
  pageTitle: "My Settings · LangWatch",
  heading: "Settings",
  subtitle: "Manage your personal API keys and view your admin-managed budget",
  backToSettings: "Back to settings",
  profile: "Profile",
  name: "Name",
  email: "Email",
  managedByIT: "Managed by {organization} IT",
  joined: "Joined",
  routing: "Routing",
  managedByOrg: "managed by your org",
  personalVirtualKeys: "Personal Virtual Keys",
  personalVirtualKeysDescription:
    "These keys let your CLI tools (Claude Code, Cursor, etc.) talk to LangWatch.",
  addNewKey: "+ Add a new key",
  newPersonalKey: "New personal key",
  newKeyPlaceholder: "e.g. jane-laptop-2",
  newKeyHint:
    "Lowercase letters, numbers, dash, underscore. The secret is shown once on creation.",
  createKey: "Create key",
  cancel: "Cancel",
  noPersonalKeys:
    "No personal keys yet. Run langwatch login in your terminal to issue your first one.",
  defaultLandingPage: "Default landing page",
  defaultLandingPageDescription:
    "Where to land when you open LangWatch. Auto uses your detected persona.",
  personalOtlpEndpoint: "Personal OTLP Endpoint",
  personalOtlpEndpointDescription:
    "Send raw OTLP traces directly to your personal workspace. For tool-specific auto-shape (Claude Code, Cursor, etc.), use the Trace Ingest tile catalog on /me when available.",
  workspaceFeatures: "Workspace features",
  workspaceFeaturesDescription:
    "Evaluations, datasets, annotations, and automations are powerful for personal projects too — turn them on when you're ready. Disabling later hides the sidebar entries; existing data is preserved.",
  enableAdvancedFeatures:
    "Enable advanced features (evaluations, datasets, annotations, automations)",
  personalBudget: "Personal budget",
  noPersonalBudget: "No personal budget set by your admin.",
  askAdminForBudget: "If you'd like one, ask your admin.",
  monthlyLimit: "Monthly limit",
  setByAdmin: "Set by your {organization} admin · cannot edit",
  currentSpend: "Current spend",
  lastUsed: "Last used {when}",
  created: "Created {when}",
  revoke: "Revoke",
  revokeConfirmation:
    "Revoke this key? Any tool using it will start failing immediately.",
  confirmRevoke: "Confirm revoke",
  newKeyCreated: "New key '{label}' created",
  dismiss: "Dismiss",
  copySecretNow: "Copy the secret now — you won't be able to see it again.",
  secretCopied: "Secret copied to clipboard",
  copy: "Copy",
  gatewayBaseUrl: "Gateway base URL",
  saveHomePreferenceError: "Couldn't save your home preference",
  auto: "Auto",
  autoDescription: "Use my detected persona — recommended",
  personalHome: "Personal home",
  personalHomeDescription: "Always land on /me",
  projectHome: "Project home",
  projectHomeDescription: "Always land on /{project}/traces",
  governanceOverview: "AI Governance overview",
  governanceOverviewDescription: "Always land on the org bird's-eye dashboard",
  saving: "Saving…",
  endpoint: "Endpoint",
  copiedToClipboard: "{label} copied to clipboard",
  apiKey: "API key",
  hide: "Hide",
  show: "Show",
  envVars: "Env vars",
  customTelemetryDescription:
    "For ad-hoc / custom telemetry. Spans land as-emitted; cost / tokens / model are not auto-populated unless your spans already follow gen_ai.* conventions. For tool-specific auto-shape, use the catalog tiles on /me when available.",
  avatarFormatHint: "PNG, JPG, WEBP or GIF, up to 8 MB. Cropped to a square.",
  profilePhoto: "Profile photo",
  savePhoto: "Save photo",
  remove: "Remove",
  changePhoto: "Change photo",
  uploadPhoto: "Upload photo",
  photoUpdated: "Photo updated",
  updatePhotoError: "Couldn't update photo",
  photoRemoved: "Photo removed",
  removePhotoError: "Couldn't remove photo",
  readImageError: "Couldn't read that image",
  editProfilePhoto: "Edit profile photo",
  addProfilePhoto: "Add profile photo",
} as const;

export type PersonalSettingsMessageKey = keyof typeof legacyMessages;

const barenaEnglishMessages: Record<PersonalSettingsMessageKey, string> = {
  ...legacyMessages,
  pageTitle: "Agent Connection · Catena",
  heading: "Agent Connection",
  subtitle:
    "Connect local Agent tools, manage credentials, and configure OTLP.",
  personalVirtualKeysDescription:
    "Use personal keys to connect CLI Agents and send authenticated activity to Catena.",
  noPersonalKeys:
    "No personal keys yet. Create one to connect your first CLI Agent.",
  defaultLandingPageDescription:
    "Choose where Catena opens after you sign in. Auto uses your detected persona.",
  personalOtlpEndpointDescription:
    "Send raw OTLP traces directly to this personal workspace. Use the Agent catalog for runtime-specific setup when available.",
  customTelemetryDescription:
    "For custom telemetry. Spans are retained as emitted; cost, token, and model fields appear when the spans follow gen_ai.* conventions.",
};

const barenaChineseMessages: Record<PersonalSettingsMessageKey, string> = {
  never: "从未",
  secondsAgo: "{count} 秒前",
  minutesAgo: "{count} 分钟前",
  hoursAgo: "{count} 小时前",
  daysAgo: "{count} 天前",
  issuedPersonalKey: "个人密钥“{label}”已创建",
  issuePersonalKeyError: "无法创建个人密钥",
  advancedFeaturesEnabled: "高级功能已启用",
  advancedFeaturesEnabledDescription:
    "评测、数据集、标注与自动化功能现已可用。",
  enableAdvancedFeaturesError: "无法启用高级功能",
  advancedFeaturesDisabled: "高级功能已关闭",
  advancedFeaturesDisabledDescription:
    "相关入口已隐藏，已有数据会保留，重新启用后即可恢复。",
  disableAdvancedFeaturesError: "无法关闭高级功能",
  keyRevoked: "密钥已撤销",
  keyRevokedDescription: "使用该密钥的 CLI 或工具将立即停止工作。",
  revokeKeyError: "无法撤销密钥",
  pageTitle: "Agent 接入 · Catena",
  heading: "Agent 接入",
  subtitle: "连接本地 Agent 工具、管理访问凭据并配置 OTLP。",
  backToSettings: "返回设置",
  profile: "个人信息",
  name: "名称",
  email: "邮箱",
  managedByIT: "由 {organization} IT 管理",
  joined: "加入时间",
  routing: "路由策略",
  managedByOrg: "由组织管理",
  personalVirtualKeys: "个人访问密钥",
  personalVirtualKeysDescription:
    "使用个人密钥连接 CLI Agent，并将经过身份验证的活动发送到 Catena。",
  addNewKey: "+ 新建密钥",
  newPersonalKey: "新建个人密钥",
  newKeyPlaceholder: "例如：xiaoba-local",
  newKeyHint: "仅支持小写字母、数字、短横线和下划线；密钥只显示一次。",
  createKey: "创建密钥",
  cancel: "取消",
  noPersonalKeys: "暂无个人密钥。新建一个密钥以连接首个 CLI Agent。",
  defaultLandingPage: "默认首页",
  defaultLandingPageDescription:
    "选择登录 Catena 后默认进入的页面；自动模式会根据使用方式判断。",
  personalOtlpEndpoint: "个人 OTLP 接入点",
  personalOtlpEndpointDescription:
    "将原始 OTLP Trace 直接发送到当前个人工作区；如需 Runtime 专属配置，请使用 Agent 目录。",
  workspaceFeatures: "工作区高级功能",
  workspaceFeaturesDescription:
    "按需启用评测、数据集、标注和自动化；关闭后只隐藏入口，不会删除已有数据。",
  enableAdvancedFeatures: "启用高级功能（评测、数据集、标注、自动化）",
  personalBudget: "个人预算",
  noPersonalBudget: "管理员尚未设置个人预算。",
  askAdminForBudget: "如有需要，请联系管理员。",
  monthlyLimit: "每月额度",
  setByAdmin: "由 {organization} 管理员设置 · 不可编辑",
  currentSpend: "本月已用",
  lastUsed: "上次使用：{when}",
  created: "创建于 {when}",
  revoke: "撤销",
  revokeConfirmation: "确认撤销该密钥？正在使用它的工具会立即停止工作。",
  confirmRevoke: "确认撤销",
  newKeyCreated: "新密钥“{label}”已创建",
  dismiss: "关闭",
  copySecretNow: "请立即复制该密钥，关闭后将无法再次查看。",
  secretCopied: "密钥已复制到剪贴板",
  copy: "复制",
  gatewayBaseUrl: "网关地址",
  saveHomePreferenceError: "无法保存默认首页",
  auto: "自动",
  autoDescription: "根据使用方式自动判断（推荐）",
  personalHome: "个人首页",
  personalHomeDescription: "始终进入 /me",
  projectHome: "项目首页",
  projectHomeDescription: "始终进入 /{project}/traces",
  governanceOverview: "AI 治理总览",
  governanceOverviewDescription: "始终进入组织全局总览",
  saving: "正在保存…",
  endpoint: "接入点",
  copiedToClipboard: "{label} 已复制到剪贴板",
  apiKey: "API 密钥",
  hide: "隐藏",
  show: "显示",
  envVars: "环境变量",
  customTelemetryDescription:
    "适用于自定义遥测数据。Span 会按原样留存；遵循 gen_ai.* 约定时，系统会展示成本、Token 和模型字段。",
  avatarFormatHint: "支持 PNG、JPG、WEBP 或 GIF，最大 8 MB，并会裁剪为正方形。",
  profilePhoto: "头像",
  savePhoto: "保存头像",
  remove: "移除",
  changePhoto: "更换头像",
  uploadPhoto: "上传头像",
  photoUpdated: "头像已更新",
  updatePhotoError: "无法更新头像",
  photoRemoved: "头像已移除",
  removePhotoError: "无法移除头像",
  readImageError: "无法读取该图片",
  editProfilePhoto: "编辑头像",
  addProfilePhoto: "添加头像",
};

export type PersonalSettingsTranslator = (
  key: PersonalSettingsMessageKey,
  variables?: Record<string, string | number>,
) => string;

function interpolate(
  message: string,
  variables?: Record<string, string | number>,
) {
  if (!variables) return message;
  return message.replace(/\{([^}]+)\}/g, (match, key: string) =>
    variables[key] === undefined ? match : String(variables[key]),
  );
}

export function translatePersonalSettingsMessage({
  locale,
  isBarenaMode,
  key,
  variables,
}: {
  locale: "en" | "zh-CN";
  isBarenaMode: boolean;
  key: PersonalSettingsMessageKey;
  variables?: Record<string, string | number>;
}) {
  const catalog = isBarenaMode
    ? locale === "zh-CN"
      ? barenaChineseMessages
      : barenaEnglishMessages
    : legacyMessages;
  return interpolate(catalog[key], variables);
}

export function usePersonalSettingsI18n() {
  const { locale } = useBarenaI18n();
  const publicEnv = usePublicEnv();
  const isBarenaMode = publicEnv.data?.IS_BARENA_MODE ?? false;

  const t = useCallback<PersonalSettingsTranslator>(
    (key, variables) =>
      translatePersonalSettingsMessage({
        locale,
        isBarenaMode,
        key,
        variables,
      }),
    [isBarenaMode, locale],
  );

  return {
    isBarenaMode,
    locale: isBarenaMode ? locale : ("en" as const),
    t,
  };
}
