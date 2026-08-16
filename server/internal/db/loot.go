package db

import (
	"database/sql"
	"time"

	"edi/internal/models"
)

// Loot tuning. Every quest completion rolls once for a drop; a dropless
// streak of pityAfter guarantees the next one (per-user counter, in-tx).
const (
	dropChance = 0.25
	pityAfter  = 6
)

// rarity thresholds on a single [0,1) roll: common 60%, uncommon 25%,
// rare 10%, epic 4%, legendary 1%.
func rollRarity(roll float64) string {
	switch {
	case roll < 0.60:
		return "common"
	case roll < 0.85:
		return "uncommon"
	case roll < 0.95:
		return "rare"
	case roll < 0.99:
		return "epic"
	default:
		return "legendary"
	}
}

// lootCatalog defines what can drop, per rarity. Effects:
//   - kind "trophy": pure collectible.
//   - kind "buff":   +Percent% XP on Attribute (”” = all) until local midnight,
//     auto-active from the moment it drops.
//   - kind "gold":   an instant gold cache (auditable gold_events row).
type lootDef struct {
	Key       string
	Name      string
	Icon      string // emoji for feeds/chat
	Kind      string // trophy | buff | gold
	Percent   int    // buff strength
	Attribute string // buff target ("" = all)
	Gold      int64  // gold-cache size
	Flavor    string
}

var lootCatalog = map[string][]lootDef{
	"common": {
		{Key: "rusty_cog", Name: "Rusty Cog", Icon: "⚙️", Kind: "trophy", Flavor: "It still turns. So do you."},
		{Key: "copper_cache", Name: "Copper Cache", Icon: "🪙", Kind: "gold", Gold: 5, Flavor: "Every run pays something."},
		{Key: "phosphor_shard", Name: "Phosphor Shard", Icon: "🟢", Kind: "trophy", Flavor: "A splinter of the terminal glow."},
	},
	"uncommon": {
		{Key: "focus_lens", Name: "Focus Lens", Icon: "🔍", Kind: "buff", Percent: 20, Attribute: "focus", Flavor: "+20% Focus XP until midnight."},
		{Key: "iron_flask", Name: "Iron Flask", Icon: "🧪", Kind: "buff", Percent: 20, Attribute: "strength", Flavor: "+20% Strength XP until midnight."},
		{Key: "silver_cache", Name: "Silver Cache", Icon: "💰", Kind: "gold", Gold: 15, Flavor: "Heavier than it looks."},
	},
	"rare": {
		{Key: "scholars_quill", Name: "Scholar's Quill", Icon: "🪶", Kind: "buff", Percent: 30, Attribute: "learning", Flavor: "+30% Learning XP until midnight."},
		{Key: "heartwood_charm", Name: "Heartwood Charm", Icon: "🌿", Kind: "buff", Percent: 30, Attribute: "health", Flavor: "+30% Health XP until midnight."},
		{Key: "gilded_cache", Name: "Gilded Cache", Icon: "👑", Kind: "gold", Gold: 40, Flavor: "Someone important lost this."},
	},
	"epic": {
		{Key: "prism_of_momentum", Name: "Prism of Momentum", Icon: "🔮", Kind: "buff", Percent: 25, Attribute: "", Flavor: "+25% ALL XP until midnight."},
		{Key: "dragonhide_ledger", Name: "Dragonhide Ledger", Icon: "🐉", Kind: "gold", Gold: 100, Flavor: "The hoard acknowledges you."},
	},
	"legendary": {
		{Key: "crown_of_streaks", Name: "Crown of Streaks", Icon: "🔥", Kind: "buff", Percent: 50, Attribute: "", Flavor: "+50% ALL XP until midnight. Wear it loudly."},
	},
}

// rollLootTx decides whether this completion drops an item and, if so, writes
// the inventory row (plus buff activation / gold cache) inside the SAME tx as
// the completion — a drop can never exist without its completion or vice
// versa. Returns nil when nothing dropped.
func (s *Store) rollLootTx(tx *sql.Tx, userID, questID int64, now time.Time) (*models.ItemDrop, error) {
	// Pity: read the dropless streak in-tx; a streak of pityAfter forces a drop.
	var dropless int
	if err := tx.QueryRow(`SELECT dropless FROM loot_pity WHERE user_id = $1`, userID).Scan(&dropless); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	dropped := s.dice.Float64() < dropChance || dropless >= pityAfter
	if !dropped {
		_, err := tx.Exec(`INSERT INTO loot_pity(user_id, dropless) VALUES($1, 1)
			ON CONFLICT(user_id) DO UPDATE SET dropless = loot_pity.dropless + 1`, userID)
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO loot_pity(user_id, dropless) VALUES($1, 0)
		ON CONFLICT(user_id) DO UPDATE SET dropless = 0`, userID); err != nil {
		return nil, err
	}

	rarity := rollRarity(s.dice.Float64())
	pool := lootCatalog[rarity]
	def := pool[int(s.dice.Float64()*float64(len(pool)))%len(pool)]

	var itemID int64
	if err := tx.QueryRow(
		`INSERT INTO user_items(user_id, item_key, rarity, source_id, created_at) VALUES($1, $2, $3, $4, $5) RETURNING id`,
		userID, def.Key, rarity, questID, now).Scan(&itemID); err != nil {
		return nil, err
	}

	drop := &models.ItemDrop{
		ID: itemID, Key: def.Key, Name: def.Name, Icon: def.Icon, Rarity: rarity,
		Kind: def.Kind, Flavor: def.Flavor, Percent: def.Percent, Attribute: def.Attribute, Gold: def.Gold,
	}

	switch def.Kind {
	case "buff":
		// Auto-active until local midnight (same local-day discipline as decay).
		_, dayEnd := localDayBounds(now)
		if _, err := tx.Exec(
			`INSERT INTO user_buffs(user_id, item_key, attribute_key, percent, expires_at, created_at) VALUES($1, $2, $3, $4, $5, $6)`,
			userID, def.Key, def.Attribute, def.Percent, dayEnd, now); err != nil {
			return nil, err
		}
		drop.ExpiresAt = &dayEnd
	case "gold":
		if _, err := insertGoldEventTx(tx, userID, def.Gold, "loot", def.Name, nil, now); err != nil {
			return nil, err
		}
	}
	return drop, nil
}

// activeBuffsTx returns the user's unexpired buffs (for the award pipeline).
func activeBuffsTx(tx *sql.Tx, userID int64, now time.Time) ([]models.ActiveBuff, error) {
	rows, err := tx.Query(
		`SELECT item_key, attribute_key, percent, expires_at FROM user_buffs
		 WHERE user_id = $1 AND expires_at > $2`, userID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ActiveBuff
	for rows.Next() {
		var b models.ActiveBuff
		if err := rows.Scan(&b.ItemKey, &b.Attribute, &b.Percent, &b.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ActiveBuffs is the read-side view (dashboard).
func (s *Store) ActiveBuffs(userID int64, now time.Time) ([]models.ActiveBuff, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	return activeBuffsTx(tx, userID, now)
}

// ListItems returns the inventory, newest first, decorated from the catalog.
func (s *Store) ListItems(userID int64, limit int) ([]models.ItemDrop, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, item_key, rarity, created_at FROM user_items WHERE user_id = $1 ORDER BY id DESC LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ItemDrop
	for rows.Next() {
		var it models.ItemDrop
		var created time.Time
		if err := rows.Scan(&it.ID, &it.Key, &it.Rarity, &created); err != nil {
			return nil, err
		}
		if def, ok := lootDefByKey(it.Key); ok {
			it.Name, it.Icon, it.Kind, it.Flavor = def.Name, def.Icon, def.Kind, def.Flavor
			it.Percent, it.Attribute, it.Gold = def.Percent, def.Attribute, def.Gold
		} else {
			it.Name, it.Icon, it.Kind = it.Key, "❔", "trophy"
		}
		it.DroppedAt = created
		out = append(out, it)
	}
	return out, rows.Err()
}

func lootDefByKey(key string) (lootDef, bool) {
	for _, pool := range lootCatalog {
		for _, d := range pool {
			if d.Key == key {
				return d, true
			}
		}
	}
	return lootDef{}, false
}
