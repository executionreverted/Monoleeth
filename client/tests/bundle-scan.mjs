import { readdir, readFile, stat } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const scanRoots = [
  { label: 'dist', root: path.resolve('dist') },
  ...process.argv.slice(2).map((arg) => ({ label: arg, root: path.resolve(arg) })),
];
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
  'map_1_3',
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

for (const scanRoot of scanRoots) {
  const rootStats = await stat(scanRoot.root).catch(() => null);
  if (!rootStats?.isDirectory()) {
    const rootLabel = formatDisplayPath(scanRoot.root);
    if (scanRoot.label === 'dist') {
      console.error('Production bundle scan failed: dist/ does not exist. Run npm run build first.');
    } else {
      console.error(`Production bundle scan failed: artifact root is missing or not a directory: ${rootLabel}`);
    }
    process.exit(1);
  }
}

const violations = [];
for (const scanRoot of scanRoots) {
  const files = await listBuiltTextAndSourceMapArtifacts(scanRoot.root);
  for (const file of files) {
    const text = await readFile(file, 'utf8');
    for (const group of forbiddenSnippetGroups) {
      for (const snippet of group.snippets) {
        if (text.includes(snippet)) {
          violations.push(
            `${formatDisplayPath(scanRoot.root)}/${path.relative(scanRoot.root, file)} contains ${group.name} ${JSON.stringify(snippet)}`,
          );
        }
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

async function listBuiltTextAndSourceMapArtifacts(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const child = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listBuiltTextAndSourceMapArtifacts(child)));
      continue;
    }
    if (entry.isFile() && /\.(?:html|css|js|mjs|json|txt|map)$/i.test(entry.name)) {
      files.push(child);
    }
  }
  return files;
}

function formatDisplayPath(filePath) {
  const relativePath = path.relative(process.cwd(), filePath);
  if (relativePath && !relativePath.startsWith('..') && !path.isAbsolute(relativePath)) {
    return relativePath;
  }
  return filePath;
}
