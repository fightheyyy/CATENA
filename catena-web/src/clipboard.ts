export type ClipboardEnvironment = {
  write?: (value: string) => Promise<void>;
  fallback?: (value: string) => boolean;
};

function browserClipboardEnvironment(): ClipboardEnvironment {
  return {
    write: navigator.clipboard?.writeText
      ? (value) => navigator.clipboard.writeText(value)
      : undefined,
    fallback: (value) => {
      const field = document.createElement("textarea");
      field.value = value;
      field.setAttribute("readonly", "");
      field.style.position = "fixed";
      field.style.opacity = "0";
      document.body.appendChild(field);
      field.select();
      try {
        return document.execCommand("copy");
      } finally {
        field.remove();
      }
    },
  };
}

export async function copyText(
  value: string,
  environment: ClipboardEnvironment = browserClipboardEnvironment(),
): Promise<boolean> {
  if (environment.write) {
    try {
      await environment.write(value);
      return true;
    } catch {
      // Some embedded browsers expose the Clipboard API but deny writes.
    }
  }
  try {
    return environment.fallback?.(value) ?? false;
  } catch {
    return false;
  }
}
