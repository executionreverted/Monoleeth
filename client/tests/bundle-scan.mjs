import { readdir, readFile, stat } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const envScanRoots = (process.env.GAME_ARTIFACT_SCAN_ROOTS ?? '')
  .split(path.delimiter)
  .map((entry) => entry.trim())
  .filter(Boolean);
const scanRoots = [
  { label: 'dist', root: path.resolve('dist') },
  ...[...envScanRoots, ...process.argv.slice(2)].map((arg) => ({ label: arg, root: path.resolve(arg) })),
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
  'border_raider_drone_pool',
  'border_raider_drone_area',
  'border_raider_drone_salvage',
  'border_raider_drone_level_2',
  'border_raider_drone_hunter',
  'border_raider_drone_patrol',
  'border_raider_salvage',
];

const forbiddenSnippetGroups = [
  { name: 'fake/default fixture label or id', snippets: fakeFixtureSnippets },
  { name: 'server-only map/content id', snippets: serverOnlyContentSnippets },
];
const oversizedEntityAssetSnippets = [
  'Nebula_Vanguard',
  'Crimson_Vortex',
  'Emerald_Void_Reaver',
  'Azure_Dreadnought',
  'Obsidian_Leviathan',
  'spin_512',
];
const maxArtifactBytes = Number.parseInt(process.env.GAME_ARTIFACT_MAX_BYTES ?? String(30 * 1024 * 1024), 10);

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
  const artifactFiles = await listBuiltArtifacts(scanRoot.root);
  const totalBytes = artifactFiles.reduce((sum, file) => sum + file.size, 0);
  if (Number.isFinite(maxArtifactBytes) && maxArtifactBytes > 0 && totalBytes > maxArtifactBytes) {
    violations.push(
      `${formatDisplayPath(scanRoot.root)} total artifact size ${formatBytes(totalBytes)} exceeds ${formatBytes(maxArtifactBytes)}`,
    );
  }
  for (const file of artifactFiles) {
    const relativeName = path.relative(scanRoot.root, file.path);
    for (const snippet of oversizedEntityAssetSnippets) {
      if (relativeName.includes(snippet)) {
        violations.push(
          `${formatDisplayPath(scanRoot.root)}/${relativeName} looks like an oversized source entity asset (${snippet})`,
        );
      }
    }
  }

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

async function listBuiltArtifacts(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const child = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listBuiltArtifacts(child)));
      continue;
    }
    if (entry.isFile()) {
      const stats = await stat(child);
      files.push({ path: child, size: stats.size });
    }
  }
  return files;
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

function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

function formatDisplayPath(filePath) {
  const relativePath = path.relative(process.cwd(), filePath);
  if (relativePath && !relativePath.startsWith('..') && !path.isAbsolute(relativePath)) {
    return relativePath;
  }
  return filePath;
}
