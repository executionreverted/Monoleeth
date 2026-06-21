import type { ClientState } from '../state/types';
import { hudSelection } from './hud-selection';
import type { AuctionGrantItem, AuctionLotItem, InventoryStackItem, MarketListingItem, PremiumEntitlementItem, PremiumStockItem, ShopCategoryID, ShopEntry, ShopProductItem } from './hud-types';
import { clamp, defaultListingPrice, escapeHTML, hasPendingOp, inventoryStackListState, lockedValue, publicAuctionName, sellableInventoryStacks, uniqueNumbers, walletBalanceForCurrency } from './hud-formatters';

export function economyPanel(state: ClientState): string {
  const wallet = state.wallet;
  const catalog = state.shopCatalog;
  if (!wallet || !catalog) {
    return `
      <h2>Shop</h2>
      <div class="empty-line">Awaiting shop catalog.</div>
    `;
  }

  const categories = [...catalog.categories].sort((left, right) => left.sort_order - right.sort_order);
  if (!categories.some((category) => category.category_id === hudSelection.selectedShopCategory)) {
    hudSelection.selectedShopCategory = categories[0]?.category_id ?? 'ships';
    hudSelection.selectedShopKey = null;
  }
  const products = [...catalog.products].sort((left, right) => left.sort_order - right.sort_order);
  const counts = new Map<string, number>();
  for (const product of products) {
    counts.set(product.category_id, (counts.get(product.category_id) ?? 0) + 1);
  }
  const entries = products.filter((product) => product.category_id === hudSelection.selectedShopCategory);
  const selected = selectedShopProduct(entries);

  return `
    <h2>Shop</h2>
    <div class="economy-metrics">
      <span>CR<strong>${wallet.credits}</strong></span>
      <span>Paid<strong>${wallet.premium_paid}</strong></span>
      <span>Earned<strong>${wallet.premium_earned}</strong></span>
    </div>
    <section class="shop-console" data-shop-console="true">
      <div class="shop-categories" role="list" aria-label="Shop categories">
        ${categories.map((category) => shopCategoryButton(category.category_id, category.display_name, counts.get(category.category_id) ?? 0, hudSelection.selectedShopCategory)).join('')}
      </div>
      <div class="shop-catalog" data-shop-category-active="${escapeHTML(hudSelection.selectedShopCategory)}">
        ${
          entries.length > 0
            ? entries.map((entry) => shopProductRow(entry, selected?.product_id)).join('')
            : `<div class="empty-line">No ${escapeHTML(hudSelection.selectedShopCategory)} entries.</div>`
        }
      </div>
      <div class="shop-detail" data-shop-detail="${escapeHTML(selected?.product_id ?? '')}">
        ${selected ? shopProductDetail(selected, wallet, catalog.catalog_version, state) : '<div class="empty-line">Select a product.</div>'}
      </div>
    </section>
  `;
}

export function selectedShopProduct(entries: ShopProductItem[]): ShopProductItem | null {
  if (entries.length === 0) {
    hudSelection.selectedShopKey = null;
    hudSelection.selectedShopQuantity = 1;
    return null;
  }
  const selected = entries.find((entry) => entry.product_id === hudSelection.selectedShopKey) ?? entries[0];
  hudSelection.selectedShopKey = selected.product_id;
  return selected;
}

export function shopSections(state: ClientState): Record<ShopCategoryID, ShopEntry[]> {
  const market = state.market;
  const auction = state.auction;
  const premium = state.premium;
  if (!market || !auction || !premium) {
    return { market: [], sell: [], auction: [], premium: [] };
  }

  const marketEntries = market.listings
    .filter((listing) => listing.status === 'active' && !listing.owned_by_you)
    .map((listing): ShopEntry => ({ key: `market:${listing.listing_id}`, category: 'market', kind: 'market_listing', item: listing }));
  const ownedListings = market.listings
    .filter((listing) => listing.status === 'active' && listing.owned_by_you)
    .map((listing): ShopEntry => ({ key: `mine:${listing.listing_id}`, category: 'sell', kind: 'owned_listing', item: listing }));
  const sellableStacks = sellableInventoryStacks(state).map(
    (item): ShopEntry => ({
      key: `sell:${item.location}:${item.item_id}`,
      category: 'sell',
      kind: 'sell_stack',
      item,
      unitPrice: defaultListingPrice(item.item_id, market),
    }),
  );
  const auctionEntries = auction.lots.map(
    (lot): ShopEntry => ({ key: `auction:${lot.auction_id}`, category: 'auction', kind: 'auction_lot', item: lot }),
  );
  const stockEntries = premium.stock.map(
    (stock): ShopEntry => ({
      key: `premium-stock:${stock.period_key}`,
      category: 'premium',
      kind: 'premium_stock',
      item: stock,
      purchased: premium.purchases.some((purchase) => purchase.period_key === stock.period_key),
    }),
  );
  const entitlementEntries = premium.entitlements.map(
    (entitlement): ShopEntry => ({
      key: `premium-entitlement:${entitlement.entitlement_id}`,
      category: 'premium',
      kind: 'premium_entitlement',
      item: entitlement,
    }),
  );
  const grantEntries = auction.grants.map(
    (grant): ShopEntry => ({ key: `auction-grant:${grant.auction_id}`, category: 'premium', kind: 'auction_grant', item: grant }),
  );

  return {
    market: marketEntries,
    sell: [...ownedListings, ...sellableStacks],
    auction: auctionEntries,
    premium: [...stockEntries, ...entitlementEntries, ...grantEntries],
  };
}

export function selectedShopEntry(entries: ShopEntry[]): ShopEntry | null {
  if (entries.length === 0) {
    hudSelection.selectedShopKey = null;
    hudSelection.selectedShopQuantity = 1;
    return null;
  }
  const selected = entries.find((entry) => entry.key === hudSelection.selectedShopKey) ?? entries[0];
  hudSelection.selectedShopKey = selected.key;
  return selected;
}

export function shopCategoryButton(id: ShopCategoryID, label: string, count: number, active: ShopCategoryID): string {
  return `
    <button class="shop-category" type="button" data-action="shop-category" data-shop-category="${escapeHTML(id)}" data-active="${active === id ? 'true' : 'false'}">
      <span>${escapeHTML(label)}</span>
      <strong>${count}</strong>
    </button>
  `;
}

export function shopProductRow(product: ShopProductItem, selectedProductID: string | undefined): string {
  const meta = [product.subcategory, product.rarity, product.tier ? `T${product.tier}` : ''].filter(Boolean).join(' / ');
  const price = `${product.price.amount} ${product.price.currency_type}`;
  return `
    <button class="shop-entry shop-product" type="button" data-action="shop-select" data-shop-key="${escapeHTML(product.product_id)}" data-selected="${product.product_id === selectedProductID ? 'true' : 'false'}" data-shop-kind="shop_product">
      <span class="shop-entry__mark"></span>
      <span>
        <strong>${escapeHTML(product.display_name)}</strong>
        <em>${escapeHTML(meta || product.product_type)}</em>
      </span>
      <small>${escapeHTML(product.availability.available ? price : product.availability.locked_reason ?? 'Unavailable')}</small>
    </button>
  `;
}

export function shopEntryRow(entry: ShopEntry, selectedKey: string | undefined): string {
  return `
    <button class="shop-entry" type="button" data-action="shop-select" data-shop-key="${escapeHTML(entry.key)}" data-selected="${entry.key === selectedKey ? 'true' : 'false'}" data-shop-kind="${escapeHTML(entry.kind)}">
      <span class="shop-entry__mark"></span>
      <span>
        <strong>${escapeHTML(shopEntryTitle(entry))}</strong>
        <em>${escapeHTML(shopEntryMeta(entry))}</em>
      </span>
      <small>${escapeHTML(shopEntryState(entry))}</small>
    </button>
  `;
}

export function shopProductDetail(product: ShopProductItem, wallet: NonNullable<ClientState['wallet']>, catalogVersion: string, state: ClientState): string {
  const balance = walletBalanceForCurrency(wallet, product.price.currency_type);
  const maxQuantity = shopProductMaxQuantity(product);
  hudSelection.selectedShopQuantity = Math.round(clamp(hudSelection.selectedShopQuantity, 1, maxQuantity));
  const total = product.price.amount * hudSelection.selectedShopQuantity;
  const canAfford = balance !== null && balance >= total;
  const pending = Object.values(state.pendingCommands).some((command) => command.op === 'shop.buy_product');
  const stock = shopProductStockLabel(product);
  const status = product.availability.available
    ? pending
      ? 'Pending'
      : canAfford
        ? 'Available'
        : 'Insufficient balance'
    : product.availability.locked_reason ?? 'Unavailable';
  const canBuy = product.availability.available && canAfford && !pending;
  return `
    <article class="shop-detail-card shop-product-detail" data-shop-detail-kind="shop-product">
      ${shopDetailHeader(product.subcategory || product.product_type, product.display_name, product.rarity || `T${product.tier ?? 1}`)}
      <div class="shop-showcase" data-art-key="${escapeHTML(product.art_key)}">
        <div class="shop-showcase__glyph">${escapeHTML(shopProductGlyph(product.product_type))}</div>
        <div>
          <strong>${escapeHTML(product.display_name)}</strong>
          <span>${escapeHTML(product.description)}</span>
        </div>
      </div>
      <div class="shop-detail-grid">
        ${shopFact('Price', `${product.price.amount} ${product.price.currency_type}`)}
        ${shopFact('Total', `${total} ${product.price.currency_type}`)}
        ${shopFact('Stock', stock)}
        ${shopFact('Status', status)}
        ${shopFact('Catalog', catalogVersion || 'current')}
        ${product.availability.required_rank ? shopFact('Rank', `${product.availability.required_rank}`) : ''}
      </div>
      ${
        product.availability.available
          ? `${maxQuantity > 1 ? shopQuantityControls(maxQuantity, hudSelection.selectedShopQuantity) : ''}
             <button class="shop-primary-action" type="button" data-action="shop-buy-product" data-product-id="${escapeHTML(product.product_id)}" data-quantity="${hudSelection.selectedShopQuantity}" ${canBuy ? '' : 'disabled'} title="${escapeHTML(canBuy ? 'Purchase selected product' : status)}">Buy</button>`
          : `<div class="shop-action-note">${escapeHTML(product.availability.locked_reason ?? 'Purchase unavailable.')}</div>`
      }
    </article>
  `;
}

export function shopProductMaxQuantity(product: ShopProductItem): number {
  if (product.product_type !== 'item') {
    return 1;
  }
  if (product.stock.kind === 'limited') {
    return Math.max(1, Math.min(9, Math.round(product.stock.stock_remaining ?? 1)));
  }
  return 9;
}

export function shopProductStockLabel(product: ShopProductItem): string {
  if (product.stock.kind === 'unlimited') {
    return 'Open';
  }
  if (product.stock.kind === 'limited') {
    const remaining = product.stock.stock_remaining ?? 0;
    const total = product.stock.stock_total ?? remaining;
    return `${remaining}/${total}`;
  }
  return 'Unavailable';
}

export function shopProductGlyph(productType: string): string {
  switch (productType) {
    case 'ship':
      return 'SHIP';
    case 'module':
      return 'MOD';
    case 'item':
      return 'MAT';
    case 'premium':
      return 'X';
    default:
      return 'ITEM';
  }
}

export function shopEntryTitle(entry: ShopEntry): string {
  switch (entry.kind) {
    case 'market_listing':
    case 'owned_listing':
      return entry.item.display_name || entry.item.item_id;
    case 'sell_stack':
      return entry.item.display_name || entry.item.item_id;
    case 'auction_lot':
      return publicAuctionName(entry.item.payload_type, entry.item.definition_id);
    case 'premium_entitlement':
      return entry.item.type.replace(/_/g, ' ');
    case 'premium_stock':
      return `Weekly ${entry.item.period_key.replace(/_/g, ' ')}`;
    case 'auction_grant':
      return publicAuctionName(entry.item.payload_type, entry.item.definition_id);
  }
}

export function shopEntryMeta(entry: ShopEntry): string {
  switch (entry.kind) {
    case 'market_listing':
      return `${entry.item.remaining_quantity} left / ${entry.item.unit_price} ${entry.item.currency_type}`;
    case 'owned_listing':
      return `${entry.item.remaining_quantity} escrowed / ${entry.item.unit_price} ${entry.item.currency_type}`;
    case 'sell_stack':
      return `${entry.item.quantity} stored / ask ${entry.unitPrice} credits`;
    case 'auction_lot':
      return `${entry.item.quantity} lot / bid ${entry.item.current_bid} ${entry.item.currency_type}`;
    case 'premium_entitlement':
      return entry.item.state;
    case 'premium_stock':
      return `${entry.item.stock_remaining}/${entry.item.stock_total} stock`;
    case 'auction_grant':
      return `${entry.item.quantity} grant / ${entry.item.reason}`;
  }
}

export function shopEntryState(entry: ShopEntry): string {
  switch (entry.kind) {
    case 'market_listing':
      return entry.item.rarity || entry.item.status || 'market';
    case 'owned_listing':
      return 'mine';
    case 'sell_stack':
      return entry.item.location.replace(/_/g, ' ');
    case 'auction_lot':
      return entry.item.leading ? 'leading' : entry.item.status || 'auction';
    case 'premium_entitlement':
      return entry.item.state;
    case 'premium_stock':
      return entry.purchased ? 'bought' : entry.item.payment_currency;
    case 'auction_grant':
      return 'grant';
  }
}

export function shopDetail(entry: ShopEntry, state: ClientState): string {
  switch (entry.kind) {
    case 'market_listing':
      return shopMarketDetail(entry.item, state.wallet, state);
    case 'owned_listing':
      return shopOwnedListingDetail(entry.item, state);
    case 'sell_stack':
      return shopSellDetail(entry.item, entry.unitPrice, state);
    case 'auction_lot':
      return shopAuctionDetail(entry.item, state.wallet, state);
    case 'premium_entitlement':
      return shopPremiumEntitlementDetail(entry.item, state);
    case 'premium_stock':
      return shopPremiumStockDetail(entry.item, entry.purchased, state.wallet, state);
    case 'auction_grant':
      return shopAuctionGrantDetail(entry.item);
  }
}

export function shopMarketDetail(listing: MarketListingItem, wallet: ClientState['wallet'], state: ClientState): string {
  const balance = wallet ? walletBalanceForCurrency(wallet, listing.currency_type) : null;
  const affordable = balance !== null && listing.unit_price > 0 ? Math.floor(balance / listing.unit_price) : listing.remaining_quantity;
  const maxQuantity = Math.max(1, Math.min(listing.remaining_quantity, Math.max(0, affordable)));
  const quantity = normalizeShopQuantity(maxQuantity);
  const estimatedSubtotal = listing.unit_price * quantity;
  const pending = hasPendingOp(state, 'market.buy');
  const canBuy =
    listing.status === 'active' &&
    !listing.owned_by_you &&
    listing.remaining_quantity >= quantity &&
    balance !== null &&
    balance >= estimatedSubtotal &&
    !pending;
  return `
    <article class="shop-detail-card" data-shop-detail-kind="market">
      ${shopDetailHeader('Market Listing', listing.display_name || listing.item_id, listing.rarity || listing.status)}
      <div class="shop-detail-grid">
        ${shopFact('Stock', `${listing.remaining_quantity}`)}
        ${shopFact('Unit', `${listing.unit_price} ${listing.currency_type}`)}
        ${shopFact('Estimate', `${estimatedSubtotal} ${listing.currency_type}`)}
        ${shopFact('Quote', pending ? 'pending' : listing.final_price_pending ? 'finalized on buy' : 'ready')}
      </div>
      ${shopQuantityControls(maxQuantity, quantity)}
      <button class="shop-primary-action" type="button" data-action="market-buy" data-listing-id="${escapeHTML(listing.listing_id)}" data-quantity="${quantity}" ${canBuy ? '' : 'disabled'} title="${escapeHTML(pending ? 'Market buy pending.' : 'Buy the selected quantity')}">Buy</button>
    </article>
  `;
}

export function shopOwnedListingDetail(listing: MarketListingItem, state: ClientState): string {
  const pending = hasPendingOp(state, 'market.cancel');
  return `
    <article class="shop-detail-card" data-shop-detail-kind="owned-listing">
      ${shopDetailHeader('Your Listing', listing.display_name || listing.item_id, listing.status)}
      <div class="shop-detail-grid">
        ${shopFact('Escrow', `${listing.remaining_quantity}`)}
        ${shopFact('Unit', `${listing.unit_price} ${listing.currency_type}`)}
        ${shopFact('State', pending ? 'pending' : listing.final_price_pending ? 'escrow held' : 'quoted')}
        ${shopFact('Listing', listing.listing_id)}
      </div>
      <button class="shop-primary-action" type="button" data-action="market-cancel" data-listing-id="${escapeHTML(listing.listing_id)}" ${pending ? 'disabled' : ''} title="${escapeHTML(pending ? 'Market cancel pending.' : 'Cancel this listing')}">Cancel</button>
    </article>
  `;
}

export function shopSellDetail(item: InventoryStackItem, unitPrice: number, state: ClientState): string {
  const maxQuantity = Math.max(1, item.quantity);
  const quantity = normalizeShopQuantity(maxQuantity);
  const pending = hasPendingOp(state, 'market.create_listing');
  const eligible = item.list_eligible === true;
  if (!eligible) {
    const listState = inventoryStackListState(item);
    return `
      <article class="shop-detail-card" data-shop-detail-kind="sell" data-list-eligible="false">
        ${shopDetailHeader('Sell Item', item.display_name || item.item_id, item.location.replace(/_/g, ' '))}
        <div class="shop-detail-grid">
          ${shopFact('Owned', `${item.quantity}`)}
          ${shopFact('Status', listState)}
        </div>
        <div class="shop-action-note">${escapeHTML(listState)}</div>
      </article>
    `;
  }
  return `
    <article class="shop-detail-card" data-shop-detail-kind="sell" data-list-eligible="true">
      ${shopDetailHeader('Sell Item', item.display_name || item.item_id, item.location.replace(/_/g, ' '))}
      <div class="shop-detail-grid">
        ${shopFact('Owned', `${item.quantity}`)}
        ${shopFact('Ask', `${unitPrice} credits`)}
        ${shopFact('Estimate', `${unitPrice * quantity} credits${pending ? ' pending' : ''}`)}
        ${shopFact('Escrow', 'held on listing')}
      </div>
      ${shopQuantityControls(maxQuantity, quantity)}
      <button class="shop-primary-action" type="button"
        data-action="market-create"
        data-item-id="${escapeHTML(item.item_id)}"
        data-source-location="${escapeHTML(item.location)}"
        data-quantity="${quantity}"
        data-unit-price="${unitPrice}"
        ${pending ? 'disabled' : ''}
        title="${escapeHTML(pending ? 'Market listing pending.' : 'Create a listing from this inventory row')}">List</button>
    </article>
  `;
}

export function shopAuctionDetail(lot: AuctionLotItem, wallet: ClientState['wallet'], state: ClientState): string {
  const bidAmount = Math.max(lot.start_price, lot.current_bid + 50);
  const balance = wallet ? walletBalanceForCurrency(wallet, lot.currency_type) : null;
  const bidPending = hasPendingOp(state, 'auction.bid');
  const buyNowPending = hasPendingOp(state, 'auction.buy_now');
  const canBid = lot.status === 'active' && balance !== null && balance >= bidAmount && !lot.leading && !bidPending;
  const canBuyNow =
    lot.status === 'active' &&
    lot.buy_now_price !== undefined &&
    balance !== null &&
    balance >= lot.buy_now_price &&
    !buyNowPending;
  return `
    <article class="shop-detail-card" data-shop-detail-kind="auction">
      ${shopDetailHeader('Auction Lot', publicAuctionName(lot.payload_type, lot.definition_id), lot.status)}
      <div class="shop-detail-grid">
        ${shopFact('Qty', `${lot.quantity}`)}
        ${shopFact('Bid', `${lot.current_bid} ${lot.currency_type}`)}
        ${shopFact('Next', `${bidAmount} ${lot.currency_type}`)}
        ${shopFact('Buy Now', lot.buy_now_price !== undefined ? `${lot.buy_now_price} ${lot.currency_type}` : lockedValue())}
        ${shopFact('Balance', balance !== null ? `${balance} ${lot.currency_type}` : lockedValue())}
      </div>
      <div class="shop-action-row">
        <button type="button" data-action="auction-bid" data-auction-id="${escapeHTML(lot.auction_id)}" data-amount="${bidAmount}" ${canBid ? '' : 'disabled'} title="${escapeHTML(bidPending ? 'Auction bid pending.' : 'Place the next bid')}">Bid</button>
        <button type="button" data-action="auction-buy-now" data-auction-id="${escapeHTML(lot.auction_id)}" ${canBuyNow ? '' : 'disabled'} title="${escapeHTML(buyNowPending ? 'Auction buy-now pending.' : 'Buy this lot now')}">Buy Now</button>
      </div>
    </article>
  `;
}

export function shopPremiumEntitlementDetail(entitlement: PremiumEntitlementItem, state: ClientState): string {
  const amount = entitlement.payload.amount ?? entitlement.payload.loadout_slot_count ?? 0;
  const pending = hasPendingOp(state, 'premium.claim');
  const canClaim = entitlement.state === 'pending' && !pending;
  return `
    <article class="shop-detail-card" data-shop-detail-kind="premium-entitlement">
      ${shopDetailHeader('Entitlement', entitlement.type.replace(/_/g, ' '), entitlement.state)}
      <div class="shop-detail-grid">
        ${shopFact('State', pending ? 'pending' : entitlement.state)}
        ${shopFact('Amount', amount > 0 ? `${amount}` : lockedValue())}
        ${shopFact('Bucket', entitlement.payload.currency_bucket ?? lockedValue())}
        ${shopFact('Created', entitlement.created_at ? `${entitlement.created_at}` : lockedValue())}
      </div>
      <button class="shop-primary-action" type="button" data-action="premium-claim" data-entitlement-id="${escapeHTML(entitlement.entitlement_id)}" ${canClaim ? '' : 'disabled'} title="${escapeHTML(pending ? 'Premium claim pending.' : 'Claim pending entitlement')}">Claim</button>
    </article>
  `;
}

export function shopPremiumStockDetail(stock: PremiumStockItem, purchased: boolean, wallet: ClientState['wallet'], state: ClientState): string {
  const balance = walletBalanceForCurrency(wallet ?? { credits: 0, premium_paid: 0, premium_earned: 0 }, stock.payment_currency);
  const pending = hasPendingOp(state, 'premium.purchase_weekly_xcore');
  const canBuy = stock.stock_remaining > 0 && !purchased && balance !== null && balance >= stock.price_amount && !pending;
  return `
    <article class="shop-detail-card" data-shop-detail-kind="premium-stock">
      ${shopDetailHeader('Premium Stock', `Weekly ${stock.period_key.replace(/_/g, ' ')}`, purchased ? 'bought' : stock.payment_currency)}
      <div class="shop-detail-grid">
        ${shopFact('Stock', `${stock.stock_remaining}/${stock.stock_total}`)}
        ${shopFact('Price', `${stock.price_amount} ${stock.payment_currency}`)}
        ${shopFact('Status', pending ? 'pending' : purchased ? 'claimed' : 'available')}
        ${shopFact('Window', purchased ? 'claimed' : 'weekly')}
      </div>
      <button class="shop-primary-action" type="button"
        data-action="premium-weekly-xcore"
        data-product-id="weekly_xcore"
        data-period-key="${escapeHTML(stock.period_key)}"
        ${canBuy ? '' : 'disabled'}
        title="${escapeHTML(pending ? 'Premium purchase pending.' : 'Purchase this weekly stock')}">Purchase</button>
    </article>
  `;
}

export function shopAuctionGrantDetail(grant: AuctionGrantItem): string {
  return `
    <article class="shop-detail-card" data-shop-detail-kind="auction-grant">
      ${shopDetailHeader('Auction Grant', publicAuctionName(grant.payload_type, grant.definition_id), grant.reason)}
      <div class="shop-detail-grid">
        ${shopFact('Qty', `${grant.quantity}`)}
        ${shopFact('Reason', grant.reason)}
        ${shopFact('Granted', `${grant.granted_at}`)}
        ${shopFact('Auction', grant.auction_id)}
      </div>
      <button class="shop-primary-action" type="button" data-action="auction-refresh" title="Refresh auction grants">Refresh</button>
    </article>
  `;
}

export function shopDetailHeader(kind: string, title: string, state: string): string {
  return `
    <div class="shop-detail-card__head">
      <span>${escapeHTML(kind)}</span>
      <strong>${escapeHTML(title)}</strong>
      <em>${escapeHTML(state || 'owned')}</em>
    </div>
  `;
}

export function shopFact(label: string, value: string): string {
  return `<div class="shop-fact"><span>${escapeHTML(label)}</span><strong>${escapeHTML(value)}</strong></div>`;
}

export function shopQuantityControls(maxQuantity: number, quantity: number): string {
  const safeMax = Math.max(1, Math.round(maxQuantity));
  const options = uniqueNumbers([1, Math.min(5, safeMax), safeMax]);
  return `
    <div class="shop-quantity" data-shop-quantity="true">
      <span>Qty</span>
      <button type="button" data-action="shop-qty" data-quantity-delta="-1" data-max-quantity="${safeMax}" ${quantity <= 1 ? 'disabled' : ''}>-</button>
      ${options
        .map(
          (option) =>
            `<button type="button" data-action="shop-qty" data-quantity="${option}" data-max-quantity="${safeMax}" data-active="${option === quantity ? 'true' : 'false'}">${option}</button>`,
        )
        .join('')}
      <button type="button" data-action="shop-qty" data-quantity-delta="1" data-max-quantity="${safeMax}" ${quantity >= safeMax ? 'disabled' : ''}>+</button>
    </div>
  `;
}

export function normalizeShopQuantity(maxQuantity: number): number {
  hudSelection.selectedShopQuantity = Math.round(clamp(hudSelection.selectedShopQuantity, 1, Math.max(1, maxQuantity)));
  return hudSelection.selectedShopQuantity;
}
