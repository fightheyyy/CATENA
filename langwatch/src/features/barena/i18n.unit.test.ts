import { describe, expect, it } from "vitest";
import { resolveBarenaLocale, translateBarenaMessage } from "./i18n";

describe("Barena locale resolution", () => {
  it("prefers a manually persisted locale", () => {
    expect(resolveBarenaLocale("en", ["zh-CN"])).toBe("en");
    expect(resolveBarenaLocale("zh-CN", ["en-US"])).toBe("zh-CN");
  });

  it("follows the browser's primary language before a manual choice", () => {
    expect(resolveBarenaLocale(null, ["zh-CN", "en-US"])).toBe("zh-CN");
    expect(resolveBarenaLocale(null, ["en-US", "zh-CN"])).toBe("en");
  });

  it("falls back to English for unsupported locales", () => {
    expect(resolveBarenaLocale("fr", ["fr-FR"])).toBe("en");
  });
});

describe("Barena message translation", () => {
  it("translates product copy into Chinese", () => {
    expect(translateBarenaMessage("zh-CN", "Release gates")).toBe("发布门禁");
    expect(translateBarenaMessage("zh-CN", "My Usage")).toBe("我的用量");
    expect(translateBarenaMessage("zh-CN", "Configure")).toBe("接入配置");
  });

  it("interpolates variables in both locales", () => {
    expect(
      translateBarenaMessage(
        "zh-CN",
        "{runId} is now producing release evidence.",
        { runId: "run-42" },
      ),
    ).toBe("run-42 正在生成发布证据。");

    expect(
      translateBarenaMessage(
        "en",
        "{runId} is now producing release evidence.",
        { runId: "run-42" },
      ),
    ).toBe("run-42 is now producing release evidence.");
  });
});
