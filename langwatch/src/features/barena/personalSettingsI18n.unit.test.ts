import { describe, expect, it } from "vitest";
import { translatePersonalSettingsMessage } from "./personalSettingsI18n";

describe("personal settings localization", () => {
  it("renders the complete Catena connection surface in Chinese", () => {
    expect(
      translatePersonalSettingsMessage({
        locale: "zh-CN",
        isBarenaMode: true,
        key: "personalVirtualKeysDescription",
      }),
    ).toContain("Catena");
    expect(
      translatePersonalSettingsMessage({
        locale: "zh-CN",
        isBarenaMode: true,
        key: "backToSettings",
      }),
    ).toBe("返回设置");
    expect(
      translatePersonalSettingsMessage({
        locale: "zh-CN",
        isBarenaMode: true,
        key: "managedByIT",
        variables: { organization: "Acme" },
      }),
    ).toBe("由 Acme IT 管理");
  });

  it("uses Catena branding for the platform English locale", () => {
    expect(
      translatePersonalSettingsMessage({
        locale: "en",
        isBarenaMode: true,
        key: "pageTitle",
      }),
    ).toBe("Agent Connection · Catena");
    expect(
      translatePersonalSettingsMessage({
        locale: "en",
        isBarenaMode: true,
        key: "heading",
      }),
    ).toBe("Agent Connection");
  });

  it("preserves legacy LangWatch copy when Barena mode is disabled", () => {
    expect(
      translatePersonalSettingsMessage({
        locale: "zh-CN",
        isBarenaMode: false,
        key: "pageTitle",
      }),
    ).toBe("My Settings · LangWatch");
    expect(
      translatePersonalSettingsMessage({
        locale: "zh-CN",
        isBarenaMode: false,
        key: "heading",
      }),
    ).toBe("Settings");
  });
});
