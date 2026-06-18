import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const sourceRoot = path.resolve('src');
const allowedFiles = new Set([
  path.join(sourceRoot, 'protocol', 'envelope.ts'),
  path.join(sourceRoot, 'protocol', 'commands.ts'),
]);
const forbiddenPatterns = [
  /\battacker_id\b/,
  /\bclient_player_id\b/,
  /\bplayer_id\s*:/,
  /\bdamage\s*:/,
  /\bxp\s*:/,
  /\bcooldown\s*:/,
  /\bgameplay_seed\b/,
  /\bfuture_spawn(?:_data)?\b/,
  /\bloot_table\b/,
];

const files = await listTSFiles(sourceRoot);
const violations = [];
for (const file of files) {
  if (allowedFiles.has(file)) {
    continue;
  }
  if (file.endsWith('.test.ts')) {
    continue;
  }
  const text = await readFile(file, 'utf8');
  for (const pattern of forbiddenPatterns) {
    if (pattern.test(text)) {
      violations.push(`${path.relative(process.cwd(), file)} matched ${pattern}`);
    }
  }
}

if (violations.length > 0) {
  console.error('Client trust-boundary lint failed:');
  for (const violation of violations) {
    console.error(`- ${violation}`);
  }
  process.exit(1);
}

async function listTSFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const child = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listTSFiles(child)));
      continue;
    }
    if (entry.isFile() && child.endsWith('.ts')) {
      files.push(child);
    }
  }
  return files;
}
