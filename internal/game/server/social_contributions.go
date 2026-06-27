package server

import (
	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/social"
)

func (runtime *Runtime) recordSocialNPCKillContributionsLocked(event combat.NPCKilledEvent, contributions map[foundation.PlayerID]float64) ([]social.ContributionSnapshot, error) {
	if runtime.SocialContributions == nil {
		return nil, nil
	}
	return runtime.SocialContributions.RecordNPCContributions(social.RecordNPCContributionInput{
		NPCEntityID:   event.NPCEntityID,
		NPCType:       event.NPCType,
		Contributions: contributions,
		OccurredAt:    event.KilledAt,
	})
}

func (runtime *Runtime) queueSocialContributionSnapshotsLocked(snapshots []social.ContributionSnapshot) {
	for _, snapshot := range snapshots {
		switch snapshot.ScopeKind {
		case social.ContributionScopeParty:
			if runtime.SocialParty == nil {
				continue
			}
			party, ok := runtime.SocialParty.Party(social.PartyID(snapshot.ScopeID))
			if !ok {
				continue
			}
			runtime.queuePartyEventLocked(party, realtime.EventPartyContribution, snapshot)
		case social.ContributionScopeClan:
			runtime.queueClanEventLocked(social.ClanID(snapshot.ScopeID), realtime.EventClanContribution, snapshot)
		}
	}
}
