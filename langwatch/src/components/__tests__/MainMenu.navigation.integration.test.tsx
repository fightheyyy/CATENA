/**
 * @vitest-environment jsdom
 *
 * @see specs/evaluations/experiments-online-evaluations-separation.feature
 */
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

const publicEnv = vi.hoisted(() => ({ data: {} as Record<string, boolean> }));

vi.mock("~/utils/compat/next-router", () => ({
  useRouter: () => ({ pathname: "/[project]" }),
}));

vi.mock("~/hooks/useOrganizationTeamProject", () => ({
  useOrganizationTeamProject: () => ({
    project: { id: "project-1", slug: "demo" },
    organization: { id: "organization-1" },
    hasPermission: () => true,
    isPublicRoute: false,
  }),
}));

vi.mock("~/hooks/useFeatureFlag", () => ({
  useFeatureFlag: () => ({ enabled: true }),
}));

vi.mock("~/hooks/useOpsPermission", () => ({
  useOpsPermission: () => ({ hasAccess: false }),
}));

vi.mock("~/hooks/usePublicEnv", () => ({
  usePublicEnv: () => publicEnv,
}));

vi.mock("~/utils/api", () => ({
  api: {
    annotation: {
      getPendingItemsCount: { useQuery: () => ({ data: 0 }) },
    },
    ops: {
      getBadgeCounts: { useQuery: () => ({ data: undefined }) },
      getDashboardSnapshot: { useQuery: () => ({ data: undefined }) },
    },
    user: {
      isAdmin: { useQuery: () => ({ data: { isAdmin: false } }) },
    },
  },
}));

vi.mock("~/components/messages/HeaderButtons", () => ({
  useTableView: () => ({ isTableView: false }),
}));

vi.mock("~/components/sidebar/CollapsibleMenuGroup", () => ({
  CollapsibleMenuGroup: ({ label }: { label: string }) => (
    <a href="/demo/simulations" aria-label={label}>
      {label}
    </a>
  ),
}));

vi.mock("~/components/sidebar/SideMenuLink", () => ({
  MENU_ITEM_HEIGHT: "32px",
  ICON_SIZE: 16,
  SideMenuLink: ({ label, href }: { label: string; href: string }) => (
    <a href={href} aria-label={label}>
      {label}
    </a>
  ),
}));

vi.mock("~/components/sidebar/UsageIndicator", () => ({
  UsageIndicator: () => null,
}));

vi.mock("~/components/sidebar/SupportMenu", () => ({
  SupportMenu: () => null,
}));

vi.mock("~/components/sidebar/ThemeToggle", () => ({
  ThemeToggle: () => null,
}));

import { MainMenu } from "../MainMenu";

const Wrapper = ({ children }: { children: React.ReactNode }) => (
  <ChakraProvider value={defaultSystem}>{children}</ChakraProvider>
);

const visibleLinkLabels = () =>
  screen.getAllByRole("link").map((link) => link.textContent);

describe("<MainMenu /> navigation", () => {
  afterEach(() => {
    cleanup();
    localStorage.clear();
    publicEnv.data = {};
  });

  it("exposes only the Spiral product allowlist in Barena mode", () => {
    publicEnv.data = { IS_BARENA_MODE: true };
    render(<MainMenu isCompact />, { wrapper: Wrapper });

    expect(visibleLinkLabels()).toEqual([
      "Home",
      "Agents",
      "Trace",
      "Explore",
      "Evolution",
      "API Keys",
      "Settings",
    ]);
    expect(screen.queryByLabelText("Language")).toBeNull();
    expect(screen.queryByLabelText("Theme")).toBeNull();
    expect(screen.queryByText("Analytics")).toBeNull();
    expect(screen.queryByText("Prompts")).toBeNull();
    expect(screen.queryByText("My Usage")).toBeNull();
  });

  /** @scenario Organize the existing destinations around the product lifecycle */
  it("uses the approved section names and destination order", () => {
    render(<MainMenu />, { wrapper: Wrapper });

    const sectionControls = screen
      .getAllByRole("button", { name: /^(Collapse|Expand) / })
      .map((button) => button.getAttribute("aria-label"));

    expect(sectionControls).toEqual([
      "Collapse Observe",
      "Collapse Test",
      "Expand Build",
      "Expand Govern",
    ]);

    expect(visibleLinkLabels()).toEqual([
      "Home",
      "Analytics",
      "Trace Explorer",
      "Traces",
      "Online Evals",
      "Evolution",
      "Explore",
      "Experiments",
      "Annotations",
      "Settings",
    ]);
  });

  /** @scenario Use sensible section defaults without a saved preference */
  it("reveals the Build destinations in their existing order", async () => {
    const user = userEvent.setup();
    render(<MainMenu />, { wrapper: Wrapper });

    expect(screen.queryByRole("link", { name: "Prompts" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Expand Build" }));

    const labels = visibleLinkLabels();
    const libraryStart = labels.indexOf("Prompts");
    expect(labels.slice(libraryStart, libraryStart + 6)).toEqual([
      "Prompts",
      "Agents",
      "Workflows",
      "Evaluators",
      "Datasets",
      "Automations",
    ]);
  });
});
