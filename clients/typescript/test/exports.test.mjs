import { beforeAll, describe, expect, it } from 'vitest';
import { build } from 'esbuild';
import { existsSync, mkdirSync, mkdtempSync, symlinkSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

// These tests resolve the package by its name, as a consumer does, so they
// read package.json's entry points rather than the source (issue 46). They
// need `npm run build` first, since the entry points name files in dist.

const pkgRoot = resolve(import.meta.dirname, '..');
let consumer;

beforeAll(() => {
  for (const f of ['dist/index.mjs', 'dist/index.js', 'dist/servicekit-client.browser.js']) {
    if (!existsSync(join(pkgRoot, f))) throw new Error(`${f} is missing; run npm run build first`);
  }
  consumer = mkdtempSync(join(tmpdir(), 'servicekit-client-'));
  mkdirSync(join(consumer, 'node_modules', '@panyam'), { recursive: true });
  symlinkSync(pkgRoot, join(consumer, 'node_modules', '@panyam', 'servicekit-client'), 'dir');
});

async function importFile(name, source) {
  const file = join(consumer, name);
  writeFileSync(file, source);
  return import(pathToFileURL(file).href);
}

describe('package entry points', () => {
  it('gives a browser bundle the ES build, with its exports', async () => {
    const out = await build({
      stdin: { contents: 'export { GRPCWSClient } from "@panyam/servicekit-client";', resolveDir: consumer },
      bundle: true,
      platform: 'browser',
      format: 'esm',
      write: false,
    });
    const code = out.outputFiles[0].text;
    expect(code).not.toContain('var ServiceKit =');
    const mod = await importFile('browser-bundle.mjs', code);
    expect(typeof mod.GRPCWSClient).toBe('function');
  });

  it('gives Node import the ES build and require the CommonJS build', async () => {
    const mod = await importFile('esm.mjs', 'export * from "@panyam/servicekit-client";');
    expect(typeof mod.GRPCWSClient).toBe('function');

    const req = createRequire(join(consumer, 'cjs.js'));
    expect(req.resolve('@panyam/servicekit-client')).toBe(join(pkgRoot, 'dist', 'index.js'));
    expect(typeof req('@panyam/servicekit-client').GRPCWSClient).toBe('function');
  });

  it('still reaches the script build and package.json by path', () => {
    const req = createRequire(join(consumer, 'cjs.js'));
    expect(req.resolve('@panyam/servicekit-client/dist/servicekit-client.browser.js'))
      .toBe(join(pkgRoot, 'dist', 'servicekit-client.browser.js'));
    expect(req('@panyam/servicekit-client/package.json').name).toBe('@panyam/servicekit-client');
  });
});
