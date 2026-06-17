package quests

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

// BoardOfferCount is the Phase 07 MVP board size.
const BoardOfferCount = 10

// QuestWeightHook deterministically scores a template for the current player.
// Hooks must be pure and stable for a given snapshot/template pair; the board
// generator clamps non-positive weights to 1 before deterministic selection.
type QuestWeightHook func(PlayerQuestBoardSnapshot, QuestTemplate) int

// QuestRecentActivity stores coarse server-owned activity counts used to nudge
// deterministic board weighting away from repetitive play.
type QuestRecentActivity struct {
	Kill    int `json:"kill,omitempty"`
	Collect int `json:"collect,omitempty"`
	Craft   int `json:"craft,omitempty"`
	Scan    int `json:"scan,omitempty"`
	Build   int `json:"build,omitempty"`
	Deliver int `json:"deliver,omitempty"`
}

// PlayerQuestBoardSnapshot is the server-owned player state needed to generate
// a deterministic quest board without trusting client-authored progress.
type PlayerQuestBoardSnapshot struct {
	PlayerID         foundation.PlayerID          `json:"player_id"`
	Rank             int                          `json:"rank"`
	MainLevel        int                          `json:"main_level"`
	RoleLevels       map[progression.RoleType]int `json:"role_levels,omitempty"`
	CurrentRegion    string                       `json:"current_region,omitempty"`
	KnownPlanetCount int                          `json:"known_planet_count,omitempty"`
	OwnedPlanetCount int                          `json:"owned_planet_count,omitempty"`
	RecentActivity   QuestRecentActivity          `json:"recent_activity,omitempty"`
}

// BoardGenerationInput carries the deterministic inputs for GenerateBoard.
type BoardGenerationInput struct {
	Player    PlayerQuestBoardSnapshot `json:"player"`
	Seed      int64                    `json:"seed"`
	Catalog   QuestCatalog             `json:"-"`
	CreatedAt time.Time                `json:"created_at"`

	WeightHook QuestWeightHook `json:"-"`
}

type weightedTemplate struct {
	template QuestTemplate
	weight   int
	score    uint64
}

type generatedOfferData struct {
	TemplateID catalog.DefinitionID `json:"template_id"`
	QuestType  QuestType            `json:"quest_type"`
	Weight     int                  `json:"weight"`
	Slot       int                  `json:"slot"`
}

// GenerateBoard returns exactly BoardOfferCount deterministic, unaccepted
// offers for eligible templates. It returns ErrInsufficientEligibleTemplates
// instead of padding or repeating templates when fewer than ten templates pass
// rank and requirement filters.
func GenerateBoard(input BoardGenerationInput) ([]GeneratedBoardOffer, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	eligible, err := input.Catalog.EligibleTemplates(input.Player)
	if err != nil {
		return nil, err
	}
	if len(eligible) < BoardOfferCount {
		return nil, fmt.Errorf("eligible templates %d need %d: %w", len(eligible), BoardOfferCount, ErrInsufficientEligibleTemplates)
	}

	createdAt := input.CreatedAt.UTC()
	expiresAt := NextQuestBoardExpiry(createdAt)
	catalogFingerprint := input.Catalog.fingerprint()
	selected := selectBoardTemplates(input, eligible, catalogFingerprint)

	offers := make([]GeneratedBoardOffer, 0, BoardOfferCount)
	for slot, candidate := range selected {
		offerSeed := stableInt64(
			"quest-board-offer-seed",
			input.Player.canonicalKey(),
			strconv.FormatInt(input.Seed, 10),
			catalogFingerprint,
			candidate.template.TemplateID.String(),
			strconv.Itoa(slot),
			createdAt.Format("2006-01-02"),
		)
		generatedPayload, err := generatePayload(input.Player, candidate.template, offerSeed, candidate.weight, slot)
		if err != nil {
			return nil, err
		}
		rewardPayload := generateRewardPayload(input.Player, candidate.template, offerSeed)
		offer, err := NewGeneratedBoardOffer(
			stableOfferID(input, candidate.template, slot, catalogFingerprint),
			input.Player.PlayerID,
			candidate.template,
			generatedPayload,
			rewardPayload,
			createdAt,
			expiresAt,
		)
		if err != nil {
			return nil, err
		}
		offers = append(offers, offer)
	}
	return offers, nil
}

// Validate reports whether the generation input is complete and server-owned.
func (input BoardGenerationInput) Validate() error {
	if err := input.Player.Validate(); err != nil {
		return err
	}
	if err := input.Catalog.Validate(); err != nil {
		return err
	}
	if input.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrZeroQuestTime)
	}
	return nil
}

// Validate reports whether the snapshot has valid IDs, levels, and activity
// counts. Missing role levels are treated as level 0 during requirement checks.
func (snapshot PlayerQuestBoardSnapshot) Validate() error {
	if err := snapshot.PlayerID.Validate(); err != nil {
		return err
	}
	if err := progression.ValidateRank(snapshot.Rank); err != nil {
		return fmt.Errorf("rank %d: %w", snapshot.Rank, ErrInvalidBoardGenerationInput)
	}
	if err := progression.ValidateMainLevel(snapshot.MainLevel); err != nil {
		return fmt.Errorf("main level %d: %w", snapshot.MainLevel, ErrInvalidBoardGenerationInput)
	}
	if strings.TrimSpace(snapshot.CurrentRegion) != snapshot.CurrentRegion {
		return fmt.Errorf("current region %q: %w", snapshot.CurrentRegion, ErrInvalidBoardGenerationInput)
	}
	if snapshot.KnownPlanetCount < 0 || snapshot.OwnedPlanetCount < 0 {
		return fmt.Errorf("planet counts known=%d owned=%d: %w", snapshot.KnownPlanetCount, snapshot.OwnedPlanetCount, ErrInvalidBoardGenerationInput)
	}
	for role, level := range snapshot.RoleLevels {
		if err := role.Validate(); err != nil {
			return fmt.Errorf("role %q: %w", role, ErrInvalidBoardGenerationInput)
		}
		if err := progression.ValidateRoleLevel(level); err != nil {
			return fmt.Errorf("role %q level %d: %w", role, level, ErrInvalidBoardGenerationInput)
		}
	}
	if err := snapshot.RecentActivity.Validate(); err != nil {
		return err
	}
	return nil
}

// RoleLevel returns a player's known role level, or 0 if the role was not
// present in the snapshot.
func (snapshot PlayerQuestBoardSnapshot) RoleLevel(role progression.RoleType) int {
	if snapshot.RoleLevels == nil {
		return 0
	}
	return snapshot.RoleLevels[role]
}

// MeetsRequirements reports whether all template requirement rows are met.
func (snapshot PlayerQuestBoardSnapshot) MeetsRequirements(requirements []QuestRequirement) bool {
	for _, requirement := range requirements {
		if requirement.MinRank > 0 && snapshot.Rank < requirement.MinRank {
			return false
		}
		if requirement.MaxRank > 0 && snapshot.Rank > requirement.MaxRank {
			return false
		}
		if !requirement.Role.IsZero() && snapshot.RoleLevel(requirement.Role) < requirement.RoleLevel {
			return false
		}
	}
	return true
}

// Validate reports whether recent activity counters are non-negative.
func (activity QuestRecentActivity) Validate() error {
	for questType, count := range map[QuestType]int{
		QuestTypeKill:    activity.Kill,
		QuestTypeCollect: activity.Collect,
		QuestTypeCraft:   activity.Craft,
		QuestTypeScan:    activity.Scan,
		QuestTypeBuild:   activity.Build,
		QuestTypeDeliver: activity.Deliver,
	} {
		if count < 0 {
			return fmt.Errorf("recent activity %q=%d: %w", questType, count, ErrInvalidBoardGenerationInput)
		}
	}
	return nil
}

// CountFor returns the activity count for a quest type.
func (activity QuestRecentActivity) CountFor(questType QuestType) int {
	switch questType {
	case QuestTypeKill:
		return activity.Kill
	case QuestTypeCollect:
		return activity.Collect
	case QuestTypeCraft:
		return activity.Craft
	case QuestTypeScan:
		return activity.Scan
	case QuestTypeBuild:
		return activity.Build
	case QuestTypeDeliver:
		return activity.Deliver
	default:
		return 0
	}
}

// DominantQuestType returns the most common recent activity type.
func (activity QuestRecentActivity) DominantQuestType() (QuestType, bool) {
	order := []QuestType{QuestTypeKill, QuestTypeCollect, QuestTypeCraft, QuestTypeScan, QuestTypeBuild, QuestTypeDeliver}
	var dominant QuestType
	maxCount := 0
	for _, questType := range order {
		count := activity.CountFor(questType)
		if count > maxCount {
			dominant = questType
			maxCount = count
		}
	}
	return dominant, maxCount > 0
}

// DefaultQuestWeightHook nudges offers toward neglected role tracks and away
// from the player's most repeated recent activity. It is intentionally simple:
// it does not read wall clock, RNG, or hidden world data, so selection remains
// deterministic for the same snapshot, seed, and catalog.
func DefaultQuestWeightHook(snapshot PlayerQuestBoardSnapshot, template QuestTemplate) int {
	weight := 100

	switch template.Type {
	case QuestTypeKill:
		weight += lowRoleBonus(snapshot.RoleLevel(progression.RoleTypeCombat), 20)
	case QuestTypeScan:
		weight += lowRoleBonus(snapshot.RoleLevel(progression.RoleTypeScout), 25)
		if snapshot.OwnedPlanetCount == 0 {
			weight += 20
		}
	case QuestTypeCraft:
		weight += lowRoleBonus(snapshot.RoleLevel(progression.RoleTypeCrafting), 25)
	case QuestTypeBuild, QuestTypeDeliver:
		weight += lowRoleBonus(snapshot.RoleLevel(progression.RoleTypeConstruction), 20)
		if snapshot.OwnedPlanetCount > 0 {
			weight += 20
		} else {
			weight -= 10
		}
	case QuestTypeCollect:
		if snapshot.KnownPlanetCount == 0 {
			weight += 10
		}
	}

	if dominant, ok := snapshot.RecentActivity.DominantQuestType(); ok {
		if template.Type == dominant {
			weight -= 15
		} else {
			weight += 10
		}
	}
	if strings.TrimSpace(snapshot.CurrentRegion) != "" && template.Type == QuestTypeDeliver {
		weight += 5
	}
	return clampWeight(weight)
}

// NextQuestBoardExpiry returns the next UTC daily reset after createdAt.
func NextQuestBoardExpiry(createdAt time.Time) time.Time {
	utc := createdAt.UTC()
	year, month, day := utc.Date()
	return time.Date(year, month, day+1, 0, 0, 0, 0, time.UTC)
}

func selectBoardTemplates(input BoardGenerationInput, eligible []QuestTemplate, catalogFingerprint string) []weightedTemplate {
	hook := input.WeightHook
	if hook == nil {
		hook = DefaultQuestWeightHook
	}
	candidates := make([]weightedTemplate, 0, len(eligible))
	for _, template := range eligible {
		weight := clampWeight(hook(input.Player, template))
		score := stableUint64(
			"quest-board-selection",
			input.Player.canonicalKey(),
			strconv.FormatInt(input.Seed, 10),
			catalogFingerprint,
			template.TemplateID.String(),
			strconv.Itoa(weight),
		) / uint64(weight)
		candidates = append(candidates, weightedTemplate{
			template: template,
			weight:   weight,
			score:    score,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score < candidates[j].score
		}
		return candidates[i].template.TemplateID < candidates[j].template.TemplateID
	})
	return candidates[:BoardOfferCount]
}

func generatePayload(snapshot PlayerQuestBoardSnapshot, template QuestTemplate, offerSeed int64, weight int, slot int) (GeneratedPayload, error) {
	data, err := json.Marshal(generatedOfferData{
		TemplateID: template.TemplateID,
		QuestType:  template.Type,
		Weight:     weight,
		Slot:       slot,
	})
	if err != nil {
		return GeneratedPayload{}, err
	}
	return GeneratedPayload{
		Seed:         offerSeed,
		Objective:    cloneObjectiveSchema(template.ObjectiveSchema),
		Difficulty:   questDifficulty(snapshot),
		TargetRegion: targetRegion(snapshot.CurrentRegion),
		Data:         data,
	}, nil
}

func generateRewardPayload(snapshot PlayerQuestBoardSnapshot, template QuestTemplate, offerSeed int64) RewardPayload {
	difficulty := int64(questDifficulty(snapshot))
	variation := int64(stableUint64("quest-board-reward", strconv.FormatInt(offerSeed, 10), template.TemplateID.String()) % 25)
	grants := []RewardGrant{
		{
			Kind:     RewardKindCredits,
			Currency: economy.CurrencyBucketCredits,
			Amount:   100 + difficulty*75 + int64(snapshot.Rank)*50 + variation,
		},
		{
			Kind:   RewardKindMainXP,
			Amount: 20 + difficulty*20 + int64(snapshot.MainLevel)*5,
		},
		{
			Kind:   RewardKindRoleXP,
			Role:   rewardRoleForQuestType(template.Type),
			Amount: 15 + difficulty*15,
		},
	}
	if itemID := rewardItemForQuestType(template.Type); !itemID.IsZero() {
		grants = append(grants, RewardGrant{
			Kind:   RewardKindItem,
			ItemID: itemID,
			Amount: difficulty,
		})
	}
	return RewardPayload{Grants: grants}
}

func stableOfferID(input BoardGenerationInput, template QuestTemplate, slot int, catalogFingerprint string) foundation.QuestID {
	createdAt := input.CreatedAt.UTC()
	key := strings.Join([]string{
		"quest-board-offer-id",
		input.Player.canonicalKey(),
		strconv.FormatInt(input.Seed, 10),
		catalogFingerprint,
		template.TemplateID.String(),
		strconv.Itoa(slot),
		createdAt.Format("2006-01-02"),
	}, "|")
	return foundation.QuestID("offer_" + stableHex([]byte(key), 24))
}

func (snapshot PlayerQuestBoardSnapshot) canonicalKey() string {
	var builder strings.Builder
	builder.WriteString("player=")
	builder.WriteString(snapshot.PlayerID.String())
	builder.WriteString("|rank=")
	builder.WriteString(strconv.Itoa(snapshot.Rank))
	builder.WriteString("|main=")
	builder.WriteString(strconv.Itoa(snapshot.MainLevel))
	builder.WriteString("|region=")
	builder.WriteString(snapshot.CurrentRegion)
	builder.WriteString("|known_planets=")
	builder.WriteString(strconv.Itoa(snapshot.KnownPlanetCount))
	builder.WriteString("|owned_planets=")
	builder.WriteString(strconv.Itoa(snapshot.OwnedPlanetCount))
	for _, role := range progression.SupportedRoleTypes() {
		builder.WriteString("|role_")
		builder.WriteString(role.String())
		builder.WriteString("=")
		builder.WriteString(strconv.Itoa(snapshot.RoleLevel(role)))
	}
	builder.WriteString("|recent_kill=")
	builder.WriteString(strconv.Itoa(snapshot.RecentActivity.Kill))
	builder.WriteString("|recent_collect=")
	builder.WriteString(strconv.Itoa(snapshot.RecentActivity.Collect))
	builder.WriteString("|recent_craft=")
	builder.WriteString(strconv.Itoa(snapshot.RecentActivity.Craft))
	builder.WriteString("|recent_scan=")
	builder.WriteString(strconv.Itoa(snapshot.RecentActivity.Scan))
	builder.WriteString("|recent_build=")
	builder.WriteString(strconv.Itoa(snapshot.RecentActivity.Build))
	builder.WriteString("|recent_deliver=")
	builder.WriteString(strconv.Itoa(snapshot.RecentActivity.Deliver))
	return builder.String()
}

func questDifficulty(snapshot PlayerQuestBoardSnapshot) int {
	difficulty := snapshot.Rank
	if snapshot.MainLevel > snapshot.Rank {
		difficulty += (snapshot.MainLevel - snapshot.Rank) / 2
	}
	if difficulty < 1 {
		return 1
	}
	if difficulty > progression.MaxMVPRank {
		return progression.MaxMVPRank
	}
	return difficulty
}

func lowRoleBonus(roleLevel int, maxBonus int) int {
	switch {
	case roleLevel <= 1:
		return maxBonus
	case roleLevel == 2:
		return maxBonus / 2
	default:
		return 0
	}
}

func clampWeight(weight int) int {
	if weight < 1 {
		return 1
	}
	return weight
}

func targetRegion(region string) string {
	if strings.TrimSpace(region) == "" {
		return "unknown"
	}
	return region
}

func rewardRoleForQuestType(questType QuestType) progression.RoleType {
	switch questType {
	case QuestTypeScan:
		return progression.RoleTypeScout
	case QuestTypeCraft:
		return progression.RoleTypeCrafting
	case QuestTypeBuild, QuestTypeDeliver:
		return progression.RoleTypeConstruction
	default:
		return progression.RoleTypeCombat
	}
}

func rewardItemForQuestType(questType QuestType) foundation.ItemID {
	switch questType {
	case QuestTypeKill:
		return "iron_ore"
	case QuestTypeCollect:
		return "carbon_shards"
	case QuestTypeCraft:
		return "energy_cell"
	case QuestTypeScan:
		return "scanner_circuit"
	case QuestTypeBuild:
		return "refined_alloy"
	case QuestTypeDeliver:
		return "helium_dust"
	default:
		return ""
	}
}

func stableInt64(parts ...string) int64 {
	value := stableUint64(parts...)
	return int64(value & ^(uint64(1) << 63))
}

func stableUint64(parts ...string) uint64 {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	sum := hash.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}

func stableHex(payload []byte, length int) string {
	sum := sha256.Sum256(payload)
	encoded := hex.EncodeToString(sum[:])
	if length <= 0 || length > len(encoded) {
		return encoded
	}
	return encoded[:length]
}
