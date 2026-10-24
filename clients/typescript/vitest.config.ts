import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['src/**/*.test.ts', 'test/**/*.test.mjs'],
    exclude: ['dist/**', 'node_modules/**'],
  },
});
