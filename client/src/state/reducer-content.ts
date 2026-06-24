import {
  assertPlayerContentCatalogObjectSafe,
  JsonObject,
} from '../protocol/envelope';
import type {
  ContentCatalogSummary,
  ContentCategorySummary,
  ContentDisplayMetadata,
  ContentItemSummary,
  ContentModuleSummary,
  ContentShopProductSummary,
} from './types';
import {
  booleanField,
  isJsonObject,
  numberField,
  objectField,
  optionalRoundedNumber,
  stringField,
} from './reducer-helpers';

export function parseContentCatalogSummary(
  payload: JsonObject,
  _fallback: ContentCatalogSummary | null,
): ContentCatalogSummary {
  assertPlayerContentCatalogObjectSafe(payload);
  const categories = Array.isArray(payload.categories)
    ? payload.categories
        .filter(isJsonObject)
        .map(parseContentCategory)
        .filter((category): category is ContentCategorySummary => category !== null)
    : [];
  const items = Array.isArray(payload.items)
    ? payload.items
        .filter(isJsonObject)
        .map(parseContentItem)
        .filter((item): item is ContentItemSummary => item !== null)
    : [];
  const modules = Array.isArray(payload.modules)
    ? payload.modules
        .filter(isJsonObject)
        .map(parseContentModule)
        .filter((module): module is ContentModuleSummary => module !== null)
    : [];
  const shopProducts = Array.isArray(payload.shop_products)
    ? payload.shop_products
        .filter(isJsonObject)
        .map(parseContentShopProduct)
        .filter((product): product is ContentShopProductSummary => product !== null)
    : [];

  return {
    version: stringField(payload, 'version') ?? '',
    categories,
    items,
    modules,
    shop_products: shopProducts,
  };
}

export function hasPlayerContentCatalogPayload(payload: JsonObject): boolean {
  return objectField(payload, 'content_catalog') !== null;
}

function parseContentCategory(payload: JsonObject): ContentCategorySummary | null {
  const categoryID = stringField(payload, 'category_id') ?? '';
  const displayName = stringField(payload, 'display_name') ?? '';
  if (!categoryID || !displayName) {
    return null;
  }
  return {
    category_id: categoryID,
    display_name: displayName,
    sort_order: Math.max(0, Math.round(numberField(payload, 'sort_order') ?? 0)),
  };
}

function parseContentDisplay(payload: JsonObject | null, fallbackName: string): ContentDisplayMetadata {
  const display = payload ?? {};
  return {
    display_name: stringField(display, 'display_name') ?? fallbackName,
    description: stringField(display, 'description') ?? undefined,
    category: stringField(display, 'category') ?? undefined,
    subcategory: stringField(display, 'subcategory') ?? undefined,
    art_key: stringField(display, 'art_key') ?? undefined,
    rarity: stringField(display, 'rarity') ?? undefined,
    tier: optionalRoundedNumber(display, 'tier', undefined),
    sort_order: optionalRoundedNumber(display, 'sort_order', undefined),
  };
}

function parseContentItem(payload: JsonObject): ContentItemSummary | null {
  const itemID = stringField(payload, 'item_id') ?? '';
  if (!itemID) {
    return null;
  }
  return {
    item_id: itemID,
    display: parseContentDisplay(objectField(payload, 'display'), itemID),
    item_type: stringField(payload, 'item_type') ?? undefined,
    rarity: stringField(payload, 'rarity') ?? undefined,
    max_stack: optionalRoundedNumber(payload, 'max_stack', undefined),
    weight_units: optionalRoundedNumber(payload, 'weight_units', undefined),
    trade_flags: stringArray(payload.trade_flags),
    bind_rules: stringArray(payload.bind_rules),
  };
}

function parseContentModule(payload: JsonObject): ContentModuleSummary | null {
  const itemID = stringField(payload, 'item_id') ?? '';
  if (!itemID) {
    return null;
  }
  const energy = objectField(payload, 'energy') ?? {};
  return {
    item_id: itemID,
    display: parseContentDisplay(objectField(payload, 'display'), stringField(payload, 'name') ?? itemID),
    name: stringField(payload, 'name') ?? undefined,
    module_category: stringField(payload, 'module_category') ?? undefined,
    slot_type: stringField(payload, 'slot_type') ?? undefined,
    tier: optionalRoundedNumber(payload, 'tier', undefined),
    rarity: stringField(payload, 'rarity') ?? undefined,
    required_rank: optionalRoundedNumber(payload, 'required_rank', undefined),
    required_role_levels: Array.isArray(payload.required_role_levels)
      ? payload.required_role_levels
          .filter(isJsonObject)
          .map(parseRoleRequirement)
          .filter((requirement): requirement is { role: string; level: number } => requirement !== null)
      : [],
    stat_modifiers: Array.isArray(payload.stat_modifiers)
      ? payload.stat_modifiers
          .filter(isJsonObject)
          .map(parseStatModifier)
          .filter((modifier): modifier is { stat: string; kind: string; value: number } => modifier !== null)
      : [],
    energy: {
      activation_cost: optionalRoundedNumber(energy, 'activation_cost', undefined),
      upkeep: optionalRoundedNumber(energy, 'upkeep', undefined),
    },
    cooldowns: Array.isArray(payload.cooldowns)
      ? payload.cooldowns
          .filter(isJsonObject)
          .map(parseCooldown)
          .filter((cooldown): cooldown is { key: string; duration_ms: number } => cooldown !== null)
      : [],
    durability_max: optionalRoundedNumber(payload, 'durability_max', undefined),
    trade_flags: stringArray(payload.trade_flags),
    bind_rules: stringArray(payload.bind_rules),
    compatible_slot_types: stringArray(payload.compatible_slot_types),
    compatible_categories: stringArray(payload.compatible_categories),
  };
}

function parseContentShopProduct(payload: JsonObject): ContentShopProductSummary | null {
  const productID = stringField(payload, 'product_id') ?? '';
  if (!productID) {
    return null;
  }
  const grantTarget = objectField(payload, 'grant_target') ?? {};
  const pricePolicy = objectField(payload, 'price_policy') ?? {};
  const stockPolicy = objectField(payload, 'stock_policy') ?? {};
  const availability = objectField(payload, 'availability') ?? {};
  return {
    product_id: productID,
    product_type: stringField(payload, 'product_type') ?? undefined,
    display: parseContentDisplay(objectField(payload, 'display'), productID),
    grant_target: {
      kind: stringField(grantTarget, 'kind') ?? undefined,
      ref_id: stringField(grantTarget, 'ref_id') ?? undefined,
      quantity: optionalRoundedNumber(grantTarget, 'quantity', undefined),
    },
    price_policy: {
      currency_type: stringField(pricePolicy, 'currency_type') ?? undefined,
      amount: optionalRoundedNumber(pricePolicy, 'amount', undefined),
      fixed: booleanField(pricePolicy, 'fixed') ?? undefined,
    },
    stock_policy: {
      kind: stringField(stockPolicy, 'kind') ?? undefined,
      remaining: optionalRoundedNumber(stockPolicy, 'remaining', undefined),
      total: optionalRoundedNumber(stockPolicy, 'total', undefined),
    },
    availability: {
      available: booleanField(availability, 'available') ?? false,
      locked_reason: stringField(availability, 'locked_reason') ?? undefined,
      required_rank: optionalRoundedNumber(availability, 'required_rank', undefined),
    },
  };
}

function parseRoleRequirement(payload: JsonObject): { role: string; level: number } | null {
  const role = stringField(payload, 'role') ?? '';
  const level = Math.max(0, Math.round(numberField(payload, 'level') ?? 0));
  return role && level > 0 ? { role, level } : null;
}

function parseStatModifier(payload: JsonObject): { stat: string; kind: string; value: number } | null {
  const stat = stringField(payload, 'stat') ?? '';
  const kind = stringField(payload, 'kind') ?? '';
  const value = numberField(payload, 'value');
  return stat && kind && value !== null ? { stat, kind, value: Math.round(value) } : null;
}

function parseCooldown(payload: JsonObject): { key: string; duration_ms: number } | null {
  const key = stringField(payload, 'key') ?? '';
  const durationMS = Math.max(0, Math.round(numberField(payload, 'duration_ms') ?? 0));
  return key && durationMS > 0 ? { key, duration_ms: durationMS } : null;
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : [];
}
