import { CLIENT_EVENTS, JsonObject } from '../protocol/envelope';
import type {
  AuctionGrantSummary,
  AuctionLotSummary,
  AuctionSummary,
  EconomyDashboardSummary,
  MarketListingSummary,
  MarketSummary,
  PremiumEntitlementSummary,
  PremiumPurchaseSummary,
  PremiumStockSummary,
  PremiumSummary,
  ShopCatalogSummary,
  ShopCategorySummary,
  ShopProductSummary,
} from './types';
import {
  booleanField,
  isJsonObject,
  numberField,
  objectField,
  optionalRoundedNumber,
  stringField,
} from './reducer-helpers';

export function parseShopCatalogSummary(payload: JsonObject, fallback: ShopCatalogSummary | null): ShopCatalogSummary {
  const categories = Array.isArray(payload.categories)
    ? payload.categories
        .filter(isJsonObject)
        .map(parseShopCategory)
        .filter((category): category is ShopCategorySummary => category !== null)
    : fallback?.categories ?? [];
  const products = Array.isArray(payload.products)
    ? payload.products
        .filter(isJsonObject)
        .map(parseShopProduct)
        .filter((product): product is ShopProductSummary => product !== null)
    : fallback?.products ?? [];
  return {
    catalog_version: stringField(payload, 'catalog_version') ?? fallback?.catalog_version ?? '',
    categories,
    products,
  };
}

function parseShopCategory(payload: JsonObject): ShopCategorySummary | null {
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

function parseShopProduct(payload: JsonObject): ShopProductSummary | null {
  const productID = stringField(payload, 'product_id') ?? '';
  const displayName = stringField(payload, 'display_name') ?? '';
  const categoryID = stringField(payload, 'category_id') ?? '';
  const artKey = stringField(payload, 'art_key') ?? '';
  if (!productID || !displayName || !categoryID || !artKey) {
    return null;
  }
  const grantTarget = objectField(payload, 'grant_target') ?? {};
  const price = objectField(payload, 'price') ?? {};
  const stock = objectField(payload, 'stock') ?? {};
  const availability = objectField(payload, 'availability') ?? {};
  return {
    product_id: productID,
    product_type: stringField(payload, 'product_type') ?? '',
    display_name: displayName,
    description: stringField(payload, 'description') ?? '',
    category_id: categoryID,
    subcategory: stringField(payload, 'subcategory') ?? undefined,
    art_key: artKey,
    rarity: stringField(payload, 'rarity') ?? undefined,
    tier: optionalRoundedNumber(payload, 'tier', undefined),
    sort_order: Math.max(0, Math.round(numberField(payload, 'sort_order') ?? 0)),
    grant_target: {
      kind: stringField(grantTarget, 'kind') ?? '',
      ref_id: stringField(grantTarget, 'ref_id') ?? '',
      quantity: optionalRoundedNumber(grantTarget, 'quantity', undefined),
    },
    price: {
      currency_type: stringField(price, 'currency_type') ?? 'credits',
      amount: Math.max(0, Math.round(numberField(price, 'amount') ?? 0)),
      fixed: booleanField(price, 'fixed') ?? true,
    },
    stock: {
      kind: stringField(stock, 'kind') ?? 'unavailable',
      stock_remaining: optionalRoundedNumber(stock, 'stock_remaining', undefined),
      stock_total: optionalRoundedNumber(stock, 'stock_total', undefined),
    },
    availability: {
      available: booleanField(availability, 'available') ?? false,
      locked_reason: stringField(availability, 'locked_reason') ?? undefined,
      required_rank: optionalRoundedNumber(availability, 'required_rank', undefined),
    },
  };
}

export function parseMarketSummary(payload: JsonObject, fallback: MarketSummary | null): MarketSummary {
  const listings = Array.isArray(payload.listings)
    ? payload.listings
        .filter(isJsonObject)
        .map(parseMarketListing)
        .filter((listing): listing is MarketListingSummary => listing !== null)
    : fallback?.listings ?? [];
  const counts = objectField(payload, 'counts') ?? {};
  return {
    listings,
    counts: {
      active: Math.max(0, Math.round(numberField(counts, 'active') ?? fallback?.counts.active ?? 0)),
      mine: Math.max(0, Math.round(numberField(counts, 'mine') ?? fallback?.counts.mine ?? 0)),
    },
  };
}

function parseMarketListing(payload: JsonObject): MarketListingSummary | null {
  const listingID = stringField(payload, 'listing_id') ?? '';
  const itemID = stringField(payload, 'item_id') ?? '';
  if (!listingID || !itemID) {
    return null;
  }
  const estimate = objectField(payload, 'estimated_unit_purchase') ?? {};
  return {
    listing_id: listingID,
    item_id: itemID,
    display_name: stringField(payload, 'display_name') ?? itemID,
    rarity: stringField(payload, 'rarity') ?? '',
    remaining_quantity: Math.max(0, Math.round(numberField(payload, 'remaining_quantity') ?? 0)),
    unit_price: Math.max(0, Math.round(numberField(payload, 'unit_price') ?? 0)),
    currency_type: stringField(payload, 'currency_type') ?? 'credits',
    status: stringField(payload, 'status') ?? '',
    expires_at: optionalRoundedNumber(payload, 'expires_at', undefined),
    owned_by_you: booleanField(payload, 'owned_by_you') ?? false,
    final_price_pending:
      booleanField(payload, 'final_price_pending') ?? booleanField(payload, 'server_recalculates') ?? true,
    estimated_unit_purchase: {
      quantity: Math.max(0, Math.round(numberField(estimate, 'quantity') ?? 0)),
      subtotal: Math.max(0, Math.round(numberField(estimate, 'subtotal') ?? 0)),
      currency_type: stringField(estimate, 'currency_type') ?? stringField(payload, 'currency_type') ?? 'credits',
      pending: booleanField(estimate, 'pending') ?? true,
    },
  };
}

export function parseAuctionSummary(payload: JsonObject, fallback: AuctionSummary | null): AuctionSummary {
  const lots = Array.isArray(payload.lots)
    ? payload.lots
        .filter(isJsonObject)
        .map(parseAuctionLot)
        .filter((lot): lot is AuctionLotSummary => lot !== null)
    : fallback?.lots ?? [];
  const grants = Array.isArray(payload.grants)
    ? payload.grants
        .filter(isJsonObject)
        .map(parseAuctionGrant)
        .filter((grant): grant is AuctionGrantSummary => grant !== null)
    : fallback?.grants ?? [];
  return { lots, grants };
}

function parseAuctionLot(payload: JsonObject): AuctionLotSummary | null {
  const auctionID = stringField(payload, 'auction_id') ?? '';
  if (!auctionID) {
    return null;
  }
  return {
    auction_id: auctionID,
    payload_type: stringField(payload, 'payload_type') ?? '',
    definition_id: stringField(payload, 'definition_id') ?? '',
    quantity: Math.max(0, Math.round(numberField(payload, 'quantity') ?? 0)),
    currency_type: stringField(payload, 'currency_type') ?? 'credits',
    start_price: Math.max(0, Math.round(numberField(payload, 'start_price') ?? 0)),
    current_bid: Math.max(0, Math.round(numberField(payload, 'current_bid') ?? 0)),
    has_bid: booleanField(payload, 'has_bid') ?? false,
    leading: booleanField(payload, 'leading') ?? false,
    buy_now_price: optionalRoundedNumber(payload, 'buy_now_price', undefined),
    status: stringField(payload, 'status') ?? '',
    starts_at: Math.max(0, Math.round(numberField(payload, 'starts_at') ?? 0)),
    ends_at: Math.max(0, Math.round(numberField(payload, 'ends_at') ?? 0)),
    final_price_pending:
      booleanField(payload, 'final_price_pending') ?? booleanField(payload, 'server_recalculates') ?? true,
  };
}

function parseAuctionGrant(payload: JsonObject): AuctionGrantSummary | null {
  const auctionID = stringField(payload, 'auction_id') ?? '';
  if (!auctionID) {
    return null;
  }
  return {
    auction_id: auctionID,
    payload_type: stringField(payload, 'payload_type') ?? '',
    definition_id: stringField(payload, 'definition_id') ?? '',
    quantity: Math.max(0, Math.round(numberField(payload, 'quantity') ?? 0)),
    reason: stringField(payload, 'reason') ?? '',
    granted_at: Math.max(0, Math.round(numberField(payload, 'granted_at') ?? 0)),
  };
}

export function parsePremiumSummary(payload: JsonObject, fallback: PremiumSummary | null): PremiumSummary {
  const entitlements = Array.isArray(payload.entitlements)
    ? payload.entitlements
        .filter(isJsonObject)
        .map(parsePremiumEntitlement)
        .filter((entitlement): entitlement is PremiumEntitlementSummary => entitlement !== null)
    : fallback?.entitlements ?? [];
  const stock = Array.isArray(payload.stock)
    ? payload.stock
        .filter(isJsonObject)
        .map(parsePremiumStock)
        .filter((record): record is PremiumStockSummary => record !== null)
    : fallback?.stock ?? [];
  const purchases = Array.isArray(payload.purchases)
    ? payload.purchases
        .filter(isJsonObject)
        .map(parsePremiumPurchase)
        .filter((purchase): purchase is PremiumPurchaseSummary => purchase !== null)
    : fallback?.purchases ?? [];
  return { entitlements, stock, purchases };
}

function parsePremiumEntitlement(payload: JsonObject): PremiumEntitlementSummary | null {
  const entitlementID = stringField(payload, 'entitlement_id') ?? '';
  const grant = objectField(payload, 'payload') ?? {};
  if (!entitlementID) {
    return null;
  }
  return {
    entitlement_id: entitlementID,
    type: stringField(payload, 'type') ?? '',
    state: stringField(payload, 'state') ?? '',
    payload: {
      currency_bucket: stringField(grant, 'currency_bucket') ?? undefined,
      amount: optionalRoundedNumber(grant, 'amount', undefined),
      loadout_slot_scope: stringField(grant, 'loadout_slot_scope') ?? undefined,
      loadout_slot_count: optionalRoundedNumber(grant, 'loadout_slot_count', undefined),
      period_key: stringField(grant, 'period_key') ?? undefined,
      cosmetic_id: stringField(grant, 'cosmetic_id') ?? undefined,
      badge_id: stringField(grant, 'badge_id') ?? undefined,
    },
    created_at: Math.max(0, Math.round(numberField(payload, 'created_at') ?? 0)),
    claimed_at: optionalRoundedNumber(payload, 'claimed_at', undefined),
  };
}

function parsePremiumStock(payload: JsonObject): PremiumStockSummary | null {
  const periodKey = stringField(payload, 'period_key') ?? '';
  if (!periodKey) {
    return null;
  }
  return {
    period_key: periodKey,
    stock_total: Math.max(0, Math.round(numberField(payload, 'stock_total') ?? 0)),
    stock_remaining: Math.max(0, Math.round(numberField(payload, 'stock_remaining') ?? 0)),
    price_amount: Math.max(0, Math.round(numberField(payload, 'price_amount') ?? 0)),
    payment_currency: stringField(payload, 'payment_currency') ?? 'premium_paid',
  };
}

function parsePremiumPurchase(payload: JsonObject): PremiumPurchaseSummary | null {
  const periodKey = stringField(payload, 'period_key') ?? '';
  if (!periodKey) {
    return null;
  }
  return {
    period_key: periodKey,
    payment_currency: stringField(payload, 'payment_currency') ?? 'premium_paid',
    granted_at: Math.max(0, Math.round(numberField(payload, 'granted_at') ?? 0)),
  };
}

export function parseEconomyDashboard(payload: JsonObject, fallback: EconomyDashboardSummary | null): EconomyDashboardSummary {
  const wallets = objectField(payload, 'wallets') ?? {};
  const market = objectField(payload, 'market') ?? {};
  const auction = objectField(payload, 'auction') ?? {};
  const premium = objectField(payload, 'premium') ?? {};
  return {
    wallets: {
      credits: Math.max(0, Math.round(numberField(wallets, 'credits') ?? fallback?.wallets.credits ?? 0)),
      premium_paid: Math.max(0, Math.round(numberField(wallets, 'premium_paid') ?? fallback?.wallets.premium_paid ?? 0)),
      premium_earned: Math.max(0, Math.round(numberField(wallets, 'premium_earned') ?? fallback?.wallets.premium_earned ?? 0)),
    },
    market: {
      active_listings: Math.max(0, Math.round(numberField(market, 'active_listings') ?? fallback?.market.active_listings ?? 0)),
      sold_listings: Math.max(0, Math.round(numberField(market, 'sold_listings') ?? fallback?.market.sold_listings ?? 0)),
      volume_credits: Math.max(0, Math.round(numberField(market, 'volume_credits') ?? fallback?.market.volume_credits ?? 0)),
    },
    auction: {
      active_lots: Math.max(0, Math.round(numberField(auction, 'active_lots') ?? fallback?.auction.active_lots ?? 0)),
      closed_lots: Math.max(0, Math.round(numberField(auction, 'closed_lots') ?? fallback?.auction.closed_lots ?? 0)),
      grants: Math.max(0, Math.round(numberField(auction, 'grants') ?? fallback?.auction.grants ?? 0)),
    },
    premium: {
      pending_entitlements: Math.max(0, Math.round(numberField(premium, 'pending_entitlements') ?? fallback?.premium.pending_entitlements ?? 0)),
      claimed_entitlements: Math.max(0, Math.round(numberField(premium, 'claimed_entitlements') ?? fallback?.premium.claimed_entitlements ?? 0)),
      weekly_stock_remaining: Math.max(0, Math.round(numberField(premium, 'weekly_stock_remaining') ?? fallback?.premium.weekly_stock_remaining ?? 0)),
    },
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

export function economyEventLog(eventType: string): string {
  switch (eventType) {
    case CLIENT_EVENTS.marketListingCreated:
      return 'Market listing created.';
    case CLIENT_EVENTS.marketSaleCompleted:
      return 'Market sale completed.';
    case CLIENT_EVENTS.marketListingCancelled:
      return 'Market listing cancelled.';
    case CLIENT_EVENTS.auctionBidPlaced:
      return 'Auction bid placed.';
    case CLIENT_EVENTS.auctionClosed:
      return 'Auction lot closed.';
    case CLIENT_EVENTS.premiumEntitlementClaimed:
      return 'Premium entitlement claimed.';
    case CLIENT_EVENTS.premiumStockConsumed:
      return 'Premium weekly stock consumed.';
    case CLIENT_EVENTS.economyFlowUpdated:
      return 'Economy flow updated.';
    default:
      return 'Economy update received.';
  }
}
