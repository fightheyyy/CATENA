/**
 * @vitest-environment jsdom
 */
import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("~/hooks/usePublicEnv", () => ({
  usePublicEnv: () => ({ data: { IS_BARENA_MODE: true } }),
}));

vi.mock("~/hooks/useFeatureFlag", () => ({
  useFeatureFlag: () => ({ enabled: true }),
}));

vi.mock("~/hooks/useOrganizationTeamProject", () => ({
  useOrganizationTeamProject: () => ({
    organization: { id: "org_1" },
    hasPermission: () => true,
  }),
}));

vi.mock("~/utils/compat/next-router", () => ({
  useRouter: () => ({ pathname: "/me" }),
}));

vi.mock("~/features/barena/i18n", () => ({
  useBarenaI18n: () => ({ t: (message: string) => message }),
}));

import { GovernSection } from "~/components/sidebar/GovernSection";

describe("given the product is running in Barena mode", () => {
  it("does not expose LangWatch Gateway or Governance in the primary rail", () => {
    const { container } = render(<GovernSection showExpanded />);

    expect(container).toBeEmptyDOMElement();
  });
});
