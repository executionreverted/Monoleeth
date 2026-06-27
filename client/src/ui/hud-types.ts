import type { ClientState } from '../state/types';
import type { RouteDestinationInput } from '../protocol/commands';
import type { JsonObject } from '../protocol/envelope';

export interface AdminContentDraftUpdateInput {
  contentType: string;
  contentID: string;
  enabled: boolean;
  displayJSON: JsonObject;
  dataJSON: JsonObject;
}

export interface HUDHandlers {
  onLogout(): void;
  onStop(): void;
  onSync(): void;
  onFire(): void;
  onLoot(): void;
  onRepairQuote(): void;
  onRepair(): void;
  onScan(): void;
  onStealthToggle(): void;
  onSelectTarget(entityID: string, source?: 'hud' | 'radar'): void;
  onCycleTarget(): void;
  onPortalEnter(portalID: string): void;
  onPlanetDetail(planetID: string): void;
  onPlanetNavigate(planetID: string): void;
  onPlanetClaim(planetID: string): void;
  onIntelShare(input: { planetID: string; toPlayerID: string }): void;
  onIntelShareToEntity(planetID: string, toEntityID: string): void;
  onCoordinateItemCreate(planetID: string): void;
  onCoordinateItemUse(itemInstanceID: string): void;
  onIntelCoordinateItemCreate(planetID: string): void;
  onIntelCoordinateItemUse(itemInstanceID: string): void;
  onPlanetBuildingBuild(input: { planetID: string; buildingType: string; slot: string }): void;
  onPlanetBuildingUpgrade(input: { planetID: string; buildingID: string; targetLevel: number }): void;
  onCraftingStart(recipeID: string, locationType?: string): void;
  onCraftingComplete(jobID: string): void;
  onCraftingCancel(jobID: string): void;
  onRouteCreate(input: { sourcePlanetID: string; destinationPlanetID?: string; destination?: RouteDestinationInput; resourceItemID: string; amountPerHour: number }): void;
  onRouteUpdate(input: { routeID: string; destinationPlanetID?: string; destination?: RouteDestinationInput; resourceItemID: string; amountPerHour: number }): void;
  onRouteEnable(routeID: string): void;
  onRouteDisable(routeID: string): void;
  onRouteSettle(routeID?: string): void;
  onHangarActivateShip(shipID: string): void;
  onLoadoutEquipModule(slotID: string, itemInstanceID: string): void;
  onLoadoutUnequipModule(slotID: string): void;
  onMarketCreateListing(input: {
    itemID: string;
    quantity: number;
    unitPrice: number;
    sourceLocation?: string;
    itemInstanceID?: string;
  }): void;
  onShopBuyProduct(productID: string, quantity: number): void;
  onMarketBuy(listingID: string, quantity: number): void;
  onMarketCancel(listingID: string): void;
  onAuctionBid(auctionID: string, amount: number): void;
  onAuctionBuyNow(auctionID: string): void;
  onAuctionGrants(): void;
  onPremiumClaim(entitlementID: string): void;
  onPremiumWeeklyXCore(productID: string, periodKey: string): void;
  onQuestAccept(offerID: string): void;
  onQuestClaim(questID: string): void;
  onQuestReroll(): void;
  onChatSend(kind: 'local_map' | 'party' | 'clan', content: string): void;
  onPartyInvite(callsign: string): void;
  onPartyAccept(inviteID: string): void;
  onPartyLeave(): void;
  onPartyTargetSet(targetID: string): void;
  onClanCreate(name: string, tag: string): void;
  onClanJoin(tag: string): void;
  onClanLeave(): void;
  onAdminRefresh(): void;
  onAdminRepairCraftJob(jobID: string): void;
  onAdminContentRefresh(): void;
  onAdminContentValidate(): void;
  onAdminContentPublish(): void;
  onAdminContentRollback(versionID: string): void;
  onAdminContentAudit(): void;
  onAdminContentUpdateDraft(input: AdminContentDraftUpdateInput): void;
}

export type EntityCombatStatus = NonNullable<ClientState['visibleEntities'][string]['combat']>;
export type KnownLootDropStatus = ClientState['knownLoot'][string];
export type VisibleEntity = ClientState['visibleEntities'][string];
export type HUDWindowID = 'cargo' | 'economy' | 'quests' | 'intel' | 'systems' | 'social' | 'ops';
export type HUDModalID = 'target' | 'planets' | 'ship' | 'planet-detail' | 'tutorial' | 'admin-content-module-edit';
export type HUDHelpTopicID = 'inventory' | 'shop' | 'quests' | 'planets' | 'hangar' | 'social' | 'ops';
export type QuickActionID = 'laser' | 'rocket' | 'scan' | 'stealth' | 'warp' | 'gather';
export type QuickActionCommand = 'fire' | 'rocket' | 'scan' | 'stealth' | 'warp' | 'loot';
export type QuestBoardSummary = NonNullable<ClientState['questBoard']>;
export type QuestOfferSummary = QuestBoardSummary['offers'][number];
export type QuestSummary = QuestBoardSummary['active'][number];
export type QuestEntry =
  | { key: string; kind: 'offer'; item: QuestOfferSummary }
  | { key: string; kind: 'quest'; item: QuestSummary };
export type ShopCategoryID = string;
export type InventoryTabID = 'equipment' | 'inventory' | 'cargo' | 'crafting';
export type ModuleFilterID = 'all' | 'offensive' | 'defensive' | 'utility';
export type InventoryStackItem = NonNullable<ClientState['inventory']>['stackable'][number];
export type ModuleInventoryItem = NonNullable<ClientState['inventory']>['instances'][number];
export type MarketListingItem = NonNullable<ClientState['market']>['listings'][number];
export type ShopProductItem = NonNullable<ClientState['shopCatalog']>['products'][number];
export type AuctionLotItem = NonNullable<ClientState['auction']>['lots'][number];
export type AuctionGrantItem = NonNullable<ClientState['auction']>['grants'][number];
export type PremiumEntitlementItem = NonNullable<ClientState['premium']>['entitlements'][number];
export type PremiumStockItem = NonNullable<ClientState['premium']>['stock'][number];
export type ShopEntry =
  | { key: string; category: 'market'; kind: 'market_listing'; item: MarketListingItem }
  | { key: string; category: 'sell'; kind: 'sell_stack'; item: InventoryStackItem; unitPrice: number }
  | { key: string; category: 'sell'; kind: 'owned_listing'; item: MarketListingItem }
  | { key: string; category: 'auction'; kind: 'auction_lot'; item: AuctionLotItem }
  | { key: string; category: 'premium'; kind: 'premium_entitlement'; item: PremiumEntitlementItem }
  | { key: string; category: 'premium'; kind: 'premium_stock'; item: PremiumStockItem; purchased: boolean }
  | { key: string; category: 'premium'; kind: 'auction_grant'; item: AuctionGrantItem };

export interface ActionState {
  enabled: boolean;
  label: string;
  detail: string;
  title: string;
}

export interface QuickActionState extends ActionState {
  id: QuickActionID;
  action: QuickActionCommand;
  slot: 1 | 2 | 3 | 4 | 5 | 6;
  key: string;
  iconURL: string;
  commandOp: string | null;
  locked: boolean;
  state: 'ready' | 'pending' | 'cooldown' | 'blocked' | 'locked' | 'scanning';
}

export interface HUDPanelDefinition {
  id: HUDWindowID;
  label: string;
  title: string;
  iconURL: string;
  helpTopic?: HUDHelpTopicID;
  render(state: ClientState): string;
  hidden?(state: ClientState): boolean;
}


export interface HUDModalState {
  id: HUDModalID;
  title: string;
  body: string;
  detailID?: string;
  helpTopic?: HUDHelpTopicID;
}

export interface HUDWindowState {
  id: HUDWindowID;
  x: number;
  y: number;
  z: number;
  open: boolean;
}

export interface HUDDragState {
  target: 'window';
  id: HUDWindowID;
  pointerID: number;
  offsetX: number;
  offsetY: number;
}

export interface HUDModalDragState {
  target: 'modal';
  pointerID: number;
  offsetX: number;
  offsetY: number;
}
