/** @vitest-environment jsdom */

import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const preferencesMocks = vi.hoisted(() => ({
  setLocale: vi.fn(),
  setTheme: vi.fn(),
}));

vi.mock("~/components/SettingsLayout", () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("~/components/HorizontalFormControl", () => ({
  HorizontalFormControl: ({
    label,
    helper,
    children,
  }: {
    label: React.ReactNode;
    helper: React.ReactNode;
    children: React.ReactNode;
  }) => (
    <section>
      <h2>{label}</h2>
      <p>{helper}</p>
      {children}
    </section>
  ),
}));

vi.mock("~/components/ui/segmented-control", () => ({
  SegmentedControl: ({
    items,
    onValueChange,
  }: {
    items: Array<{ value: string; label: React.ReactNode }>;
    onValueChange: (details: { value: string }) => void;
  }) => (
    <div>
      {items.map((item) => (
        <button
          key={item.value}
          type="button"
          onClick={() => onValueChange({ value: item.value })}
        >
          {item.label}
        </button>
      ))}
    </div>
  ),
}));

vi.mock("~/features/barena/i18n", () => ({
  useBarenaI18n: () => ({
    locale: "en",
    setLocale: preferencesMocks.setLocale,
    t: (message: string) => message,
  }),
}));

vi.mock("next-themes", () => ({
  useTheme: () => ({
    theme: "system",
    setTheme: preferencesMocks.setTheme,
  }),
}));

import PreferencesPage from "../preferences";

describe("<PreferencesPage />", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("keeps language and theme together in Settings", () => {
    render(
      <ChakraProvider value={defaultSystem}>
        <PreferencesPage />
      </ChakraProvider>,
    );

    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "Appearance & Language",
      }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Chinese/ }));
    fireEvent.click(screen.getByRole("button", { name: /Dark/ }));

    expect(preferencesMocks.setLocale).toHaveBeenCalledWith("zh-CN");
    expect(preferencesMocks.setTheme).toHaveBeenCalledWith("dark");
  });
});
