import { useColorMode } from "../ui/color-mode";
import { CatenaMark } from "./CatenaMark";

/**
 * Catena platform wordmark.
 *
 * Keep this component API-compatible with upstream LangWatch so the
 * downstream brand remains a small, reviewable patch.
 */
export function FullLogo({
  width = 176,
  height = 42,
  forceColorMode,
}: {
  width?: number | string;
  height?: number | string;
  forceColorMode?: "light" | "dark";
}) {
  const { colorMode: systemColorMode } = useColorMode();
  const colorMode = forceColorMode ?? systemColorMode;
  const foreground = colorMode === "dark" ? "#FFFFFF" : "#000000";

  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={width}
      height={height}
      fill="none"
      viewBox="0 0 176 42"
      role="img"
      aria-label="Catena Agent Evolution Platform"
    >
      <svg
        x="0"
        y="0"
        width="42"
        height="42"
        viewBox="0 0 100 100"
        color={foreground}
        aria-hidden="true"
      >
        <CatenaMark />
      </svg>
      <text
        x="49"
        y="24"
        fill={foreground}
        fontFamily="Inter, ui-sans-serif, system-ui, sans-serif"
        fontSize="20"
        fontWeight="800"
        letterSpacing="0.8"
      >
        CATENA
      </text>
      <text
        x="50"
        y="35"
        fill={foreground}
        fillOpacity="0.56"
        fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace"
        fontSize="7"
        fontWeight="700"
        letterSpacing="1.45"
      >
        AGENT EVOLUTION
      </text>
    </svg>
  );
}
