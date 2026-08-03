/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("~/components/sidebar/SideMenuLink", () => ({
  SideMenuLink: ({
    label,
    href,
    isActive,
  }: {
    label: string;
    href: string;
    isActive: boolean;
  }) => (
    <a href={href} data-active={isActive ? "true" : "false"}>
      {label}
    </a>
  ),
}));

import { BarenaUtilityNavigation } from "../BarenaUtilityNavigation";

describe("<BarenaUtilityNavigation />", () => {
  afterEach(() => cleanup());

  it("promotes API Keys next to Settings and keeps one active utility", () => {
    render(
      <BarenaUtilityNavigation
        pathname="/settings/api-keys"
        showLabel
        visible
      />,
    );

    const links = screen.getAllByRole("link");
    expect(links.map((link) => link.textContent)).toEqual([
      "API Keys",
      "Settings",
    ]);
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/settings/api-keys",
      "/settings",
    ]);
    expect(
      links
        .filter((link) => link.dataset.active === "true")
        .map((link) => link.textContent),
    ).toEqual(["API Keys"]);
  });

  it("renders no privileged utilities when settings are unavailable", () => {
    render(
      <BarenaUtilityNavigation
        pathname="/[project]"
        showLabel
        visible={false}
      />,
    );

    expect(screen.queryAllByRole("link")).toHaveLength(0);
  });
});
