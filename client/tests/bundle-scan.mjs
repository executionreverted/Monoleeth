import { readdir, readFile, stat } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const distRoot = path.resolve('dist');
const fakeFixtureSnippets = [
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

const serverOnlyContentSnippets = [
  'map_1_1',
  'map_1_2',
  'starter_training_drone_pool',
  'starter_training_drone_area',
  'training_drone_salvage',
  'outer_ring_scout_drone_pool',
  'outer_ring_scout_drone_area',
  'outer_ring_scout_drone_salvage',
  'outer_ring_scout_drone_level_1',
  'outer_ring_scout_drone_cautious',
  'outer_ring_scout_drone_patrol',
];

const forbiddenSnippetGroups = [
  { name: 'fake/default fixture label or id', snippets: fakeFixtureSnippets },
  { name: 'server-only map/content id', snippets: serverOnlyContentSnippets },
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
  for (const group of forbiddenSnippetGroups) {
    for (const snippet of group.snippets) {
      if (text.includes(snippet)) {
        violations.push(`${path.relative(process.cwd(), file)} contains ${group.name} ${JSON.stringify(snippet)}`);
      }
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
