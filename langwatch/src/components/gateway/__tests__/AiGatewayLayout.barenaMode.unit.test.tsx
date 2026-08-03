/**
 * @vitest-environment jsdom
 */
import { render, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  sectionLayout: vi.fn(),
}));

vi.mock("~/hooks/usePublicEnv", () => ({
  usePublicEnv: () => ({ data: { IS_BARENA_MODE: true } }),
}));

vi.mock("~/utils/compat/next-router", () => ({
  useRouter: () => ({ replace: mocks.replace }),
}));

vi.mock("~/components/ui/layouts/SectionNavigationLayout", () => ({
  SectionNavigationLayout: () => {
    mocks.sectionLayout();
    return null;
  },
}));

import AiGatewayLayout, {
  BARENA_API_KEYS_PATH,
} from "~/components/gateway/AiGatewayLayout";

describe("given a LangWatch AI Gateway URL is opened in Barena mode", () => {
  it("routes to the unified project API key settings instead", async () => {
    render(
      <AiGatewayLayout>
        <div>gateway</div>
      </AiGatewayLayout>,
    );

    await waitFor(() => {
      expect(mocks.replace).toHaveBeenCalledWith(BARENA_API_KEYS_PATH);
    });
    expect(mocks.sectionLayout).not.toHaveBeenCalled();
  });
});
