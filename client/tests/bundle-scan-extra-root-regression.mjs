import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const scanner = path.join(scriptDir, 'bundle-scan.mjs');
const root = await mkdtemp(path.join(tmpdir(), 'bundle-scan-extra-root-'));

try {
  await writeSafeDist(root);

  const cleanRoot = path.join(root, 'published-clean');
  await mkdir(cleanRoot, { recursive: true });
  await writeFile(path.join(cleanRoot, 'app.js'), 'window.__build = "clean";\n');
  assertRun(runScanner(root, [cleanRoot]), 0, 'clean positional extra root');

  const badPositionalRoot = path.join(root, 'published-positional');
  await mkdir(badPositionalRoot, { recursive: true });
  await writeFile(path.join(badPositionalRoot, 'app.js'), 'window.__hidden = "map_1_2";\n');
  const badPositional = runScanner(root, [badPositionalRoot]);
  assertRun(badPositional, 1, 'bad positional extra root');
  assertIncludes(badPositional.stderr, 'map_1_2', 'bad positional token');
  assertIncludes(badPositional.stderr, 'published-positional', 'bad positional root label');

  const badEnvRoot = path.join(root, 'published-env');
  await mkdir(badEnvRoot, { recursive: true });
  await writeFile(path.join(badEnvRoot, 'app.js'), 'window.__fixture = "Demo Fringe";\n');
  const badEnv = runScanner(root, [], { GAME_ARTIFACT_SCAN_ROOTS: badEnvRoot });
  assertRun(badEnv, 1, 'bad env extra root');
  assertIncludes(badEnv.stderr, 'Demo Fringe', 'bad env token');
  assertIncludes(badEnv.stderr, 'published-env', 'bad env root label');

  const badEntityRoot = path.join(root, 'published-entity-asset');
  await mkdir(path.join(badEntityRoot, 'assets'), { recursive: true });
  await writeFile(path.join(badEntityRoot, 'assets', 'Nebula_Vanguard_2_spin_512-CANARY.gif'), 'oversized entity asset');
  const badEntity = runScanner(root, [badEntityRoot]);
  assertRun(badEntity, 1, 'bad oversized entity asset root');
  assertIncludes(badEntity.stderr, 'Nebula_Vanguard', 'bad entity asset token');
  assertIncludes(badEntity.stderr, 'oversized source entity asset', 'bad entity asset reason');

  const badSizeRoot = path.join(root, 'published-too-large');
  await mkdir(badSizeRoot, { recursive: true });
  await writeFile(path.join(badSizeRoot, 'big.bin'), '0123456789abcdef');
  const badSize = runScanner(root, [badSizeRoot], { GAME_ARTIFACT_MAX_BYTES: '8' });
  assertRun(badSize, 1, 'bad artifact size root');
  assertIncludes(badSize.stderr, 'total artifact size', 'bad size reason');
  assertIncludes(badSize.stderr, 'exceeds', 'bad size threshold');

  console.log('bundle-scan extra root regression ok');
} finally {
  await rm(root, { recursive: true, force: true });
}

async function writeSafeDist(rootDir) {
  const dist = path.join(rootDir, 'dist');
  await mkdir(dist, { recursive: true });
  await writeFile(path.join(dist, 'index.html'), '<!doctype html><title>clean</title>\n');
}

function runScanner(cwd, args, env = {}) {
  return spawnSync(process.execPath, [scanner, ...args], {
    cwd,
    env: { ...process.env, ...env },
    encoding: 'utf8',
  });
}

function assertRun(result, expectedStatus, label) {
  if (result.status === expectedStatus) return;
  throw new Error(`${label} exited ${result.status}, want ${expectedStatus}\nstdout:\n${result.stdout}\nstderr:\n${result.stderr}`);
}

function assertIncludes(text, expected, label) {
  if (String(text).includes(expected)) return;
  throw new Error(`${label} missing ${JSON.stringify(expected)} in:\n${text}`);
}
