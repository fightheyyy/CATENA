import { CatenaMark } from "./CatenaMark";

export function LogoIcon({ width, height }: { width: number; height: number }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={width}
      height={height}
      fill="none"
      viewBox="0 0 100 100"
      role="img"
      aria-label="Catena"
      color="currentColor"
    >
      <CatenaMark />
    </svg>
  );
}
