import { readdir, readFile, stat } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const distRoot = path.resolve('dist');
const forbiddenSnippets = [
  'Demo Fringe',
  'Fixture Belt',
  'Frontier-01',
  'Drone Rake',
  'Scrap Cache',
  'player-local',
  'npc-rake-01',
  'loot-scrap-01',
  'signal-eris-04',
  'npc_placeholder',
  'loot_placeholder',
  'planet_signal_placeholder',
];

const distStats = await stat(distRoot).catch(() => null);
if (!distStats?.isDirectory()) {
  console.error('Production bundle scan failed: dist/ does not exist. Run npm run build first.');
  process.exit(1);
}

const files = await listBundleFiles(distRoot);
const violations = [];
for (const file of files) {
  const text = await readFile(file, 'utf8');
  for (const snippet of forbiddenSnippets) {
    if (text.includes(snippet)) {
      violations.push(`${path.relative(process.cwd(), file)} contains ${JSON.stringify(snippet)}`);
    }
  }
}

if (violations.length > 0) {
  console.error('Production bundle scan failed:');
  for (const violation of violations) {
    console.error(`- ${violation}`);
  }
  process.exit(1);
}

async function listBundleFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const child = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listBundleFiles(child)));
      continue;
    }
    if (entry.isFile() && /\.(?:html|css|js|mjs|json|txt)$/i.test(entry.name)) {
      files.push(child);
    }
  }
  return files;
}
