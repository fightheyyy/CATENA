/**
 * @vitest-environment jsdom
 *
 * Regression guard: moving from the personal workspace into organization
 * settings must not collapse the 200px primary navigation into the 56px hover
 * rail. That route-driven width jump made the left navigation look broken and
 * hid every label until hover.
 */
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const captured = vi.hoisted(() => ({
  compactMenu: undefined as unknown,
  personalScope: undefined as unknown,
}));
const publicEnv = vi.hoisted(() => ({
  data: { IS_SAAS: false, IS_BARENA_MODE: false },
}));

vi.mock("~/components/DashboardLayout", () => ({
  DashboardLayout: (props: {
    compactMenu?: boolean;
    personalScope?: boolean;
  }) => {
    captured.compactMenu = props.compactMenu;
    captured.personalScope = props.personalScope;
    return null;
  },
}));

vi.mock("~/components/MenuLink", () => ({
  MenuLink: ({
    href,
    children,
  }: {
    href: string;
    children: React.ReactNode;
  }) => <a href={href}>{children}</a>,
}));

vi.mock("~/hooks/useOrganizationTeamProject", () => ({
  useOrganizationTeamProject: () => ({ hasPermission: () => true }),
}));

vi.mock("~/hooks/usePublicEnv", () => ({
  usePublicEnv: () => publicEnv,
}));

vi.mock("~/hooks/useActivePlan", () => ({
  useActivePlan: () => ({ isEnterprise: false, isLoading: false }),
}));

vi.mock("~/hooks/useLiteMemberGuard", () => ({
  useLiteMemberGuard: () => ({ isLiteMember: false }),
}));

vi.mock("~/hooks/useOpsPermission", () => ({
  useOpsPermission: () => ({ hasAccess: false }),
}));

vi.mock("~/utils/api", () => ({
  api: {
    user: {
      isAdmin: {
        useQuery: () => ({ data: { isAdmin: false } }),
      },
    },
  },
}));

vi.mock("~/utils/compat/next-navigation", () => ({
  usePathname: () => "/settings",
}));

import SettingsLayout, {
  BarenaSettingsNavigation,
} from "~/components/SettingsLayout";

describe("given organization settings is open on desktop", () => {
  afterEach(() => {
    captured.compactMenu = undefined;
    captured.personalScope = undefined;
    publicEnv.data.IS_BARENA_MODE = false;
    vi.clearAllMocks();
  });

  it("keeps the primary navigation expanded", () => {
    render(<SettingsLayout />);

    expect(captured.compactMenu).not.toBe(true);
    expect(captured.personalScope).not.toBe(true);
  });

  it("uses personal product navigation in Barena settings", () => {
    publicEnv.data.IS_BARENA_MODE = true;
    render(<SettingsLayout />);

    expect(captured.personalScope).toBe(true);
  });

  it("keeps only the settings needed by the Spiral MVP", () => {
    render(<BarenaSettingsNavigation />);

    expect(screen.getAllByRole("link").map((link) => link.textContent)).toEqual(
      [
        "General Settings",
        "Appearance & Language",
        "Agent Connection",
        "Model Providers",
        "Members",
        "Teams & Projects",
      ],
    );
  });
});
