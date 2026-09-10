package services

import (
	"errors"
	"sort"
	"strings"

	"edi/internal/db"
	"edi/internal/models"
)

// Cosmetics: gear for the hero, bought once with gold and worn forever.
// Purely cosmetic — no stat, no XP, no decay, never lost. The catalog is
// code (like loot): every client renders from the look hints (shape,
// color, accent) so the web, the CLI and the agent describe the same hero.
//
// Empty slots fall back to the level look (sword Lv3, shield Lv6, helm
// Lv10, crown Lv15 — the free unlocks the hero always had).

// CosmeticSlots is the fixed slot order (also the wardrobe layout).
var CosmeticSlots = []string{"head", "body", "weapon", "offhand", "back", "aura", "pet"}

// cosmeticDef is a catalog entry. MinLevel gates the purchase (a progression
// hook), never the wearing: bought is bought.
type cosmeticDef struct {
	Key      string
	Slot     string
	Name     string
	Rarity   string
	Price    int64
	MinLevel int
	Shape    string
	Color    string
	Accent   string
	Flavor   string
}

var cosmeticCatalog = []cosmeticDef{
	// head
	{Key: "leather_hood", Slot: "head", Name: "Leather Hood", Rarity: "common", Price: 20, MinLevel: 1, Shape: "hood", Color: "#7a4f2a", Accent: "#5a381c", Flavor: "Keeps the rain and the doubts out."},
	{Key: "iron_helm", Slot: "head", Name: "Iron Helm", Rarity: "uncommon", Price: 45, MinLevel: 2, Shape: "helm", Color: "#9aa7b0", Accent: "#5c6770", Flavor: "Dented once. Never twice."},
	{Key: "wizard_hat", Slot: "head", Name: "Wizard Hat", Rarity: "rare", Price: 90, MinLevel: 3, Shape: "hat", Color: "#4b3fa8", Accent: "#ffd700", Flavor: "Points at the goal. Literally."},
	{Key: "dragon_helm", Slot: "head", Name: "Dragon Helm", Rarity: "epic", Price: 180, MinLevel: 6, Shape: "horned", Color: "#b3261e", Accent: "#ffb347", Flavor: "The horns are decorative. Mostly."},
	{Key: "crown_of_dawn", Slot: "head", Name: "Crown of Dawn", Rarity: "legendary", Price: 400, MinLevel: 10, Shape: "crown", Color: "#ffd700", Accent: "#ff6b6b", Flavor: "Worn by those who kept showing up."},
	// body
	{Key: "traveler_tunic", Slot: "body", Name: "Traveler Tunic", Rarity: "common", Price: 20, MinLevel: 1, Shape: "tunic", Color: "#2c6fb5", Accent: "#1b4a7a", Flavor: "Road dust included."},
	{Key: "chainmail", Slot: "body", Name: "Chainmail", Rarity: "uncommon", Price: 50, MinLevel: 2, Shape: "mail", Color: "#8f9ba5", Accent: "#4f5a63", Flavor: "Heavy, and worth it."},
	{Key: "mage_robe", Slot: "body", Name: "Mage Robe", Rarity: "rare", Price: 90, MinLevel: 3, Shape: "robe", Color: "#5a3fb8", Accent: "#c9b6ff", Flavor: "Sleeves big enough for three plans."},
	{Key: "dragonscale", Slot: "body", Name: "Dragonscale Plate", Rarity: "epic", Price: 200, MinLevel: 6, Shape: "plate", Color: "#b3261e", Accent: "#ffb347", Flavor: "Every scale a finished day."},
	{Key: "void_plate", Slot: "body", Name: "Void Plate", Rarity: "legendary", Price: 400, MinLevel: 10, Shape: "plate", Color: "#1a1030", Accent: "#9b5cff", Flavor: "Absorbs excuses."},
	// weapon
	{Key: "wooden_staff", Slot: "weapon", Name: "Wooden Staff", Rarity: "common", Price: 15, MinLevel: 1, Shape: "staff", Color: "#8b5a2b", Accent: "#3ddc84", Flavor: "A stick with intent."},
	{Key: "longsword", Slot: "weapon", Name: "Longsword", Rarity: "uncommon", Price: 45, MinLevel: 2, Shape: "sword", Color: "#d9e2e8", Accent: "#ffd700", Flavor: "Sharp enough to cut a task in half."},
	{Key: "war_axe", Slot: "weapon", Name: "War Axe", Rarity: "rare", Price: 90, MinLevel: 3, Shape: "axe", Color: "#b0bec5", Accent: "#6d4c2a", Flavor: "For the big ones."},
	{Key: "flame_blade", Slot: "weapon", Name: "Flame Blade", Rarity: "epic", Price: 180, MinLevel: 6, Shape: "sword", Color: "#ff7a1a", Accent: "#ffd166", Flavor: "Burns procrastination on contact."},
	{Key: "storm_spear", Slot: "weapon", Name: "Storm Spear", Rarity: "legendary", Price: 350, MinLevel: 10, Shape: "spear", Color: "#7fd4ff", Accent: "#ffffff", Flavor: "Strikes first. Always."},
	// offhand
	{Key: "wooden_buckler", Slot: "offhand", Name: "Wooden Buckler", Rarity: "common", Price: 15, MinLevel: 1, Shape: "buckler", Color: "#8b5a2b", Accent: "#c69c6d", Flavor: "Small, round, enough."},
	{Key: "tower_shield", Slot: "offhand", Name: "Tower Shield", Rarity: "uncommon", Price: 45, MinLevel: 2, Shape: "tower", Color: "#4f6b8f", Accent: "#ffd700", Flavor: "Nothing gets past a whole week."},
	{Key: "spell_tome", Slot: "offhand", Name: "Spell Tome", Rarity: "rare", Price: 80, MinLevel: 3, Shape: "tome", Color: "#6a1b9a", Accent: "#ffd700", Flavor: "Every page a lesson learned."},
	{Key: "mirror_shield", Slot: "offhand", Name: "Mirror Shield", Rarity: "epic", Price: 160, MinLevel: 6, Shape: "tower", Color: "#cfe8ff", Accent: "#7fd4ff", Flavor: "Reflects doubt back where it came from."},
	// back
	{Key: "red_cape", Slot: "back", Name: "Red Cape", Rarity: "common", Price: 25, MinLevel: 1, Shape: "cape", Color: "#c62828", Accent: "#8e1b1b", Flavor: "Capes make everything 20% faster."},
	{Key: "phantom_cloak", Slot: "back", Name: "Phantom Cloak", Rarity: "rare", Price: 100, MinLevel: 3, Shape: "cape", Color: "#2b2f4a", Accent: "#9b5cff", Flavor: "Half here, half already done."},
	{Key: "phoenix_wings", Slot: "back", Name: "Phoenix Wings", Rarity: "legendary", Price: 450, MinLevel: 10, Shape: "wings", Color: "#ff8c1a", Accent: "#ffd166", Flavor: "Every return is a first flight."},
	// aura
	{Key: "ember_aura", Slot: "aura", Name: "Ember Aura", Rarity: "uncommon", Price: 60, MinLevel: 2, Shape: "ring", Color: "#ff7a1a", Accent: "#ffd166", Flavor: "Warm to stand next to."},
	{Key: "frost_aura", Slot: "aura", Name: "Frost Aura", Rarity: "rare", Price: 110, MinLevel: 3, Shape: "ring", Color: "#7fd4ff", Accent: "#ffffff", Flavor: "Cool head, steady hands."},
	{Key: "void_aura", Slot: "aura", Name: "Void Aura", Rarity: "epic", Price: 220, MinLevel: 6, Shape: "orbit", Color: "#9b5cff", Accent: "#1a1030", Flavor: "Distractions fall in and never return."},
	// pet
	{Key: "slime", Slot: "pet", Name: "Pocket Slime", Rarity: "common", Price: 30, MinLevel: 1, Shape: "slime", Color: "#3ddc84", Accent: "#0b1210", Flavor: "Follows you. Judges nothing."},
	{Key: "owl", Slot: "pet", Name: "Study Owl", Rarity: "uncommon", Price: 70, MinLevel: 2, Shape: "owl", Color: "#8d6e63", Accent: "#ffd700", Flavor: "Asks 'who?' Never 'why not yet?'"},
	{Key: "wisp", Slot: "pet", Name: "Wisp", Rarity: "rare", Price: 120, MinLevel: 3, Shape: "wisp", Color: "#7fd4ff", Accent: "#ffffff", Flavor: "A small light for the late hours."},
	{Key: "baby_dragon", Slot: "pet", Name: "Baby Dragon", Rarity: "legendary", Price: 500, MinLevel: 10, Shape: "dragon", Color: "#b3261e", Accent: "#ffb347", Flavor: "Grows with you. Breathes encouragement."},
}

var cosmeticByKey = func() map[string]cosmeticDef {
	m := make(map[string]cosmeticDef, len(cosmeticCatalog))
	for _, d := range cosmeticCatalog {
		m[d.Key] = d
	}
	return m
}()

var rarityRank = map[string]int{"common": 0, "uncommon": 1, "rare": 2, "epic": 3, "legendary": 4}

func (d cosmeticDef) equipped() models.EquippedCosmetic {
	return models.EquippedCosmetic{Slot: d.Slot, Key: d.Key, Name: d.Name, Rarity: d.Rarity, Shape: d.Shape, Color: d.Color, Accent: d.Accent}
}

func (d cosmeticDef) item(level int, owned, equipped bool) models.CosmeticItem {
	return models.CosmeticItem{
		Key: d.Key, Slot: d.Slot, Name: d.Name, Rarity: d.Rarity, Price: d.Price, MinLevel: d.MinLevel,
		Shape: d.Shape, Color: d.Color, Accent: d.Accent, Flavor: d.Flavor,
		Owned: owned, Equipped: equipped, Unlocked: level >= d.MinLevel,
	}
}

// characterLevel is the aggregate level used by the wardrobe gate — the
// same formula as Dashboard.Character (computed from total XP on read).
func (s *Service) characterLevel() (int, error) {
	attrs, err := s.ListAttributes()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, a := range attrs {
		total += a.TotalXP
	}
	return LevelForXP(total), nil
}

// Loadout resolves the equipped gear to full look hints (unknown keys —
// e.g. a piece removed from the catalog — are skipped silently).
func (s *Service) Loadout() ([]models.EquippedCosmetic, error) {
	worn, err := s.store.CosmeticLoadout(s.userID)
	if err != nil {
		return nil, err
	}
	return resolveLoadout(worn), nil
}

func resolveLoadout(worn map[string]string) []models.EquippedCosmetic {
	out := make([]models.EquippedCosmetic, 0, len(worn))
	for _, slot := range CosmeticSlots {
		key, ok := worn[slot]
		if !ok {
			continue
		}
		def, ok := cosmeticByKey[key]
		if !ok || def.Slot != slot {
			continue
		}
		out = append(out, def.equipped())
	}
	return out
}

// ListCosmetics returns the wardrobe: the whole catalog annotated for this
// user, sorted by slot order then rarity then price, plus the loadout,
// balance and level (so a client can gate without a second call).
func (s *Service) ListCosmetics() (models.CosmeticCatalog, error) {
	level, err := s.characterLevel()
	if err != nil {
		return models.CosmeticCatalog{}, err
	}
	owned, err := s.store.OwnedCosmetics(s.userID)
	if err != nil {
		return models.CosmeticCatalog{}, err
	}
	worn, err := s.store.CosmeticLoadout(s.userID)
	if err != nil {
		return models.CosmeticCatalog{}, err
	}
	balance, err := s.store.GoldBalance(s.userID)
	if err != nil {
		return models.CosmeticCatalog{}, err
	}
	slotIdx := map[string]int{}
	for i, sl := range CosmeticSlots {
		slotIdx[sl] = i
	}
	items := make([]models.CosmeticItem, 0, len(cosmeticCatalog))
	for _, d := range cosmeticCatalog {
		items = append(items, d.item(level, owned[d.Key], worn[d.Slot] == d.Key))
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if slotIdx[a.Slot] != slotIdx[b.Slot] {
			return slotIdx[a.Slot] < slotIdx[b.Slot]
		}
		if rarityRank[a.Rarity] != rarityRank[b.Rarity] {
			return rarityRank[a.Rarity] < rarityRank[b.Rarity]
		}
		return a.Price < b.Price
	})
	return models.CosmeticCatalog{
		Items: items, Loadout: resolveLoadout(worn), Balance: balance, Level: level,
		Slots: append([]string(nil), CosmeticSlots...),
	}, nil
}

func lookupCosmetic(key string) (cosmeticDef, error) {
	def, ok := cosmeticByKey[strings.ToLower(strings.TrimSpace(key))]
	if !ok {
		return cosmeticDef{}, ErrNotFound
	}
	return def, nil
}

// BuyCosmetic spends gold on a catalog piece and equips it. Unknown key →
// 404; already owned, below the level requirement, or not enough gold → 400.
func (s *Service) BuyCosmetic(key string) (models.CosmeticPurchaseResult, error) {
	def, err := lookupCosmetic(key)
	if err != nil {
		return models.CosmeticPurchaseResult{}, err
	}
	level, err := s.characterLevel()
	if err != nil {
		return models.CosmeticPurchaseResult{}, err
	}
	if level < def.MinLevel {
		return models.CosmeticPurchaseResult{}, validationErr("%s needs level %d (you are level %d)", def.Name, def.MinLevel, level)
	}
	ev, balance, err := s.store.PurchaseCosmetic(s.userID, def.Key, def.Slot, def.Name, def.Price)
	switch {
	case errors.Is(err, db.ErrCosmeticOwned):
		return models.CosmeticPurchaseResult{}, validationErr("you already own %s", def.Name)
	case errors.Is(err, db.ErrInsufficientGold):
		return models.CosmeticPurchaseResult{}, validationErr("not enough gold: %s costs %dg", def.Name, def.Price)
	case err != nil:
		return models.CosmeticPurchaseResult{}, err
	}
	loadout, err := s.Loadout()
	if err != nil {
		return models.CosmeticPurchaseResult{}, err
	}
	return models.CosmeticPurchaseResult{
		Item: def.item(level, true, true), Event: ev, Balance: balance, Loadout: orEmpty(loadout),
	}, nil
}

// EquipCosmetic wears an owned piece. Unknown key → 404; not owned → 400.
func (s *Service) EquipCosmetic(key string) ([]models.EquippedCosmetic, error) {
	def, err := lookupCosmetic(key)
	if err != nil {
		return nil, err
	}
	if err := s.store.EquipCosmetic(s.userID, def.Key, def.Slot); err != nil {
		if errors.Is(err, db.ErrCosmeticNotOwned) {
			return nil, validationErr("you don't own %s yet", def.Name)
		}
		return nil, err
	}
	loadout, err := s.Loadout()
	return orEmpty(loadout), err
}

// UnequipCosmetic empties a slot (unknown slot → 400). Idempotent.
func (s *Service) UnequipCosmetic(slot string) ([]models.EquippedCosmetic, error) {
	slot = strings.ToLower(strings.TrimSpace(slot))
	if !validCosmeticSlot(slot) {
		return nil, validationErr("unknown slot %q (one of %s)", slot, strings.Join(CosmeticSlots, ", "))
	}
	if err := s.store.UnequipCosmetic(s.userID, slot); err != nil {
		return nil, err
	}
	loadout, err := s.Loadout()
	return orEmpty(loadout), err
}

func validCosmeticSlot(slot string) bool {
	for _, sl := range CosmeticSlots {
		if sl == slot {
			return true
		}
	}
	return false
}
