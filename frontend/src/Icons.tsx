import type { SVGProps } from "react";

export type IconName = "home" | "assistant" | "history" | "memory" | "outputs" | "agents" | "plug" | "settings" | "search" | "arrow" | "plus" | "refresh" | "book" | "graph" | "clock" | "close";
const paths: Record<IconName, React.ReactNode> = {
  home: <><rect x="3" y="3" width="7" height="7" rx="2" /><rect x="14" y="3" width="7" height="7" rx="2" /><rect x="3" y="14" width="7" height="7" rx="2" /><rect x="14" y="14" width="7" height="7" rx="2" /></>,
  assistant: <><path d="M12 2.5 13.7 9l6.3 1.8-6.3 1.8-1.7 6.9-1.7-6.9L4 10.8 10.3 9 12 2.5Z" /><path d="m19 17 .6 1.5L21 19l-1.4.5L19 21l-.6-1.5L17 19l1.4-.5L19 17Z" /></>,
  history: <><path d="M4 6h5m6 0h5M4 18h5m6 0h5M9 6l6 12M9 18l6-12" /><circle cx="4" cy="6" r="2" /><circle cx="20" cy="6" r="2" /><circle cx="4" cy="18" r="2" /><circle cx="20" cy="18" r="2" /></>,
  memory: <><path d="M12 20V7m0 4C4 12 3 8 4 4c4-1 8 0 8 5m0 7c8 1 10-3 9-7-4-1-9 0-9 5M8 21h8" /></>,
  outputs: <><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9zm0 0v6h6M8 13h8m-8 4h5" /></>,
  agents: <><rect x="4" y="6" width="16" height="14" rx="4" /><path d="M12 3v3M8 12v2m8-2v2M9 17h6" /></>,
  plug: <><path d="M9 3v5m6-5v5M6 8h12v3a6 6 0 0 1-6 6v4M9 21h6" /></>,
  settings: <><path d="M4 6h6m4 0h6M4 12h10m4 0h2M4 18h2m4 0h10" /><circle cx="12" cy="6" r="2" /><circle cx="16" cy="12" r="2" /><circle cx="8" cy="18" r="2" /></>,
  search: <><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4.5 4.5" /></>,
  arrow: <path d="M4 12h15m-6-6 6 6-6 6" />,
  plus: <path d="M12 5v14M5 12h14" />,
  refresh: <><path d="M20 7v5h-5M4 17v-5h5M6 6a8 8 0 0 1 13 3M5 15a8 8 0 0 0 13 3" /></>,
  book: <><path d="M12 5v16M3 4c4-1 6-1 9 1 3-2 5-2 9-1v15c-4-1-6-1-9 2-3-3-5-3-9-2z" /></>,
  graph: <><path d="m6 6 12 3M6 6l5 12m7-9-7 9" /><circle cx="6" cy="6" r="3" /><circle cx="18" cy="9" r="3" /><circle cx="11" cy="18" r="3" /></>,
  clock: <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></>,
  close: <path d="m6 6 12 12M6 18 18 6" />,
};

export function Icon({ name, ...props }: SVGProps<SVGSVGElement> & { name: IconName }) {
  return <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>{paths[name]}</svg>;
}
