/**
 * @vitest-environment jsdom
 */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../../ui/color-mode", () => ({
  useColorMode: () => ({ colorMode: "light" }),
}));

import { FullLogo } from "../FullLogo";
import { LogoIcon } from "../LogoIcon";

describe("Catena brand", () => {
  afterEach(() => cleanup());

  it("exposes the compact chain-cat mark as Catena", () => {
    const { container } = render(<LogoIcon width={42} height={42} />);

    expect(screen.getByRole("img", { name: "Catena" })).toBeInTheDocument();
    expect(container.querySelectorAll("circle")).toHaveLength(9);
    expect(container.querySelectorAll("rect")).toHaveLength(2);
    expect(container.querySelector("[fill='#E9B949']")).toBeNull();
  });

  it("uses the monochrome Catena platform wordmark", () => {
    const { container } = render(<FullLogo />);

    const wordmark = screen.getByRole("img", {
      name: "Catena Agent Evolution Platform",
    });
    expect(wordmark).toHaveTextContent("CATENA");
    expect(wordmark).toHaveTextContent("AGENT EVOLUTION");
    expect(wordmark).not.toHaveTextContent("BARENA");
    expect(wordmark).not.toHaveTextContent("SPIRAL");
    expect(container.querySelector("[fill='#E9B949']")).toBeNull();
  });
});
