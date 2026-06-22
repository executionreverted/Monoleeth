import type { ClientState, MapSummary } from '../state/types';
import { lockedValue } from './hud-formatters';

export function topbarLocationText(state: Pick<ClientState, 'currentMap' | 'sector'>): string {
  return (
    cleanText(state.currentMap?.display_name) ??
    cleanText(state.currentMap?.public_map_key) ??
    cleanText(state.currentMap?.map_key) ??
    cleanText(state.sector?.name) ??
    lockedValue()
  );
}

export function topbarDangerText(state: Pick<ClientState, 'currentMap' | 'sector'>): string {
  const mapLabel = currentMapDangerText(state.currentMap);
  if (mapLabel) {
    return mapLabel;
  }
  if (state.sector) {
    return state.sector.contested ? 'contested' : cleanText(state.sector.danger) ?? lockedValue();
  }
  return lockedValue();
}

function currentMapDangerText(map: MapSummary | null): string | null {
  if (!map) {
    return null;
  }
  if (map.protection?.blocks_pvp) {
    return 'protected';
  }
  if (map.safe_zone?.inside) {
    return map.safe_zone.blocks_pvp ? 'safe zone' : 'safe area';
  }

  const risk = cleanText(map.risk_band);
  const policy = cleanText(map.pvp_policy);
  if (risk && policy && normalizePolicyToken(risk) !== normalizePolicyToken(policy)) {
    return `${risk}/${policy}`;
  }
  return risk ?? policy;
}

function cleanText(value: string | undefined): string | null {
  const text = value?.trim();
  return text ? text : null;
}

function normalizePolicyToken(value: string): string {
  return value.trim().toLowerCase().replace(/[\s_-]+/g, '');
}
