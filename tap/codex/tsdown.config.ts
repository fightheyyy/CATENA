import { defineConfig } from "tsdown";

export default defineConfig({
  entry: ["src/index.ts"],
  outDir: "plugins/tracing/dist",
  format: ["esm"],
  platform: "node",
  target: "node22",
  dts: false,
  clean: true,
  minify: false,
  outputOptions: {
    inlineDynamicImports: true
  }
});
