export const command = "analyze-log";
export async function runLogInspection(file: string) {
  return { source: file, reviewed: true };
}
