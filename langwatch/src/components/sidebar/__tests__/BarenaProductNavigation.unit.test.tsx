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

import { BarenaProductNavigation } from "../BarenaProductNavigation";

describe("<BarenaProductNavigation />", () => {
  afterEach(() => cleanup());

  it("defines one small, ordered product shell", () => {
    render(
      <BarenaProductNavigation
        project={{ slug: "demo" }}
        pathname="/[project]/simulations/scenarios"
        showLabel
      />,
    );

    const links = screen.getAllByRole("link");
    expect(links.map((link) => link.textContent)).toEqual([
      "Home",
      "Agents",
      "Trace",
      "Explore",
      "Evolution",
    ]);
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/demo",
      "/demo/agent-registry",
      "/demo/traces",
      "/demo/simulations/scenarios",
      "/demo/evolution",
    ]);
    expect(
      links
        .filter((link) => link.dataset.active === "true")
        .map((link) => link.textContent),
    ).toEqual(["Explore"]);
  });
});
