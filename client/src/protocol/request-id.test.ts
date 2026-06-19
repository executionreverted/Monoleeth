import { expect, test } from 'vitest';

import { assertClientSafePayload, CommandBuilder } from './commands';
import { createRequestId } from './request-id';

test('createRequestId returns non-empty unique ids', () => {
  const ids = new Set(Array.from({ length: 20 }, () => createRequestId()));

  expect(ids.size).toBe(20);
  for (const id of ids) {
    expect(id.length).toBeGreaterThan(8);
  }
});

test('command builders include request ids and omit trusted fields', () => {
  const builder = new CommandBuilder();
  const move = builder.moveTo({ x: 120, y: -40 });
  const fire = builder.combatUseSkill('npc-1');
  const pickup = builder.lootPickup('drop-1');
  const scan = builder.scanPulse();
  const serializedPayloads = JSON.stringify([move.payload, fire.payload, pickup.payload, scan.payload]);

  expect(move.request_id).toBeTruthy();
  expect(move.client_seq).toBe(1);
  expect(move.payload).toEqual({ target: { x: 120, y: -40 } });
  expect(fire.op).toBe('combat.use_skill');
  expect(fire.payload).toEqual({ target_id: 'npc-1', skill_id: 'basic_laser' });
  expect(pickup.op).toBe('loot.pickup');
  expect(pickup.payload).toEqual({ drop_id: 'drop-1' });
  expect(scan.op).toBe('scan.pulse');
  expect(scan.payload).toEqual({});
  expect(serializedPayloads).not.toContain('player_id');
  expect(serializedPayloads).not.toContain('damage');
  expect(serializedPayloads).not.toContain('xp');
  expect(serializedPayloads).not.toContain('loot');
  expect(serializedPayloads).not.toContain('cooldown');
});

test('phase 09 command builders send selectors without trusted quest or admin truth', () => {
  const builder = new CommandBuilder();
  const board = builder.questBoard();
  const accept = builder.questAccept('offer-1');
  const progress = builder.questProgress();
  const claim = builder.questClaimReward('quest-1');
  const reroll = builder.questReroll();
  const inspect = builder.adminInspectPlayer();
  const repair = builder.adminRepairCraftJob('craft-job-1');
  const metrics = builder.observabilityMetrics();

  expect(board.op).toBe('quest.board');
  expect(board.payload).toEqual({});
  expect(accept.payload).toEqual({ offer_id: 'offer-1' });
  expect(progress.payload).toEqual({});
  expect(claim.payload).toEqual({ quest_id: 'quest-1' });
  expect(reroll.payload).toEqual({});
  expect(inspect.payload).toEqual({});
  expect(repair.payload).toEqual({ job_id: 'craft-job-1' });
  expect(metrics.payload).toEqual({});
});

test('economy, premium, and quest command builders omit trusted result fields', () => {
  const builder = new CommandBuilder();
  const commands = [
    builder.marketSearch('raw_ore'),
    builder.marketCreateListing({
      itemID: 'raw_ore',
      quantity: 2,
      unitPrice: 15,
      sourceLocation: 'ship_cargo',
      itemInstanceID: 'instance-1',
    }),
    builder.marketBuy('listing-1', 2),
    builder.marketCancel('listing-1'),
    builder.auctionSearch(),
    builder.auctionBid('auction-1', 50),
    builder.auctionBuyNow('auction-1'),
    builder.auctionClaimGrant(),
    builder.premiumEntitlements(),
    builder.premiumClaim('entitlement-1'),
    builder.premiumPurchaseWeeklyXCore(),
    builder.questBoard(),
    builder.questAccept('offer-1'),
    builder.questProgress(),
    builder.questClaimReward('quest-1'),
    builder.questReroll(),
  ];
  const serializedPayloads = JSON.stringify(commands.map((command) => command.payload));

  expect(commands[1].payload).toEqual({
    item_id: 'raw_ore',
    quantity: 2,
    unit_price: 15,
    source_location: 'ship_cargo',
    item_instance_id: 'instance-1',
  });
  expect(commands[5].payload).toEqual({ auction_id: 'auction-1', amount: 50 });
  expect(commands[14].payload).toEqual({ quest_id: 'quest-1' });

  for (const forbidden of [
    'player_id',
    'account_id',
    'damage',
    'xp',
    'wallet',
    'credits',
    'balance',
    'price_total',
    'seller_proceeds',
    'quest_progress',
    'objective_progress',
    'reward_payload',
    'hidden',
    'gameplay_seed',
    'loot_table',
  ]) {
    expect(serializedPayloads).not.toContain(forbidden);
  }
});

test('phase 09 command payloads reject client-authored quest truth', () => {
  expect(() => assertClientSafePayload({ progress: { current: 99 } })).toThrow(/trusted field: progress/);
  expect(() => assertClientSafePayload({ reward_payload: { credits: 1000 } })).toThrow(/trusted field: reward_payload/);
  expect(() => assertClientSafePayload({ generated_seed: 12345 })).toThrow(/trusted field: generated_seed/);
});

test('command payloads reject client-authored economy, combat, and hidden world truth', () => {
  expect(() => assertClientSafePayload({ player_id: 'player-1' })).toThrow(/trusted field: player_id/);
  expect(() => assertClientSafePayload({ damage: 20 })).toThrow(/trusted field: damage/);
  expect(() => assertClientSafePayload({ wallet_amount: 1000 })).toThrow(/trusted field: wallet_amount/);
  expect(() => assertClientSafePayload({ balance_after: 900 })).toThrow(/trusted field: balance_after/);
  expect(() => assertClientSafePayload({ quest_progress: { kill: 1 } })).toThrow(/trusted field: quest_progress/);
  expect(() => assertClientSafePayload({ hidden: true })).toThrow(/trusted field: hidden/);
  expect(() => assertClientSafePayload({ scan_result: { detected: true } })).toThrow(/trusted field: scan_result/);
  expect(() => assertClientSafePayload({ loot_table: 'rare' })).toThrow(/trusted field: loot_table/);
});
