package services

import (
	"log"
	"time"

	"edi/internal/models"
)

// Achievements: a catalog-in-code of badges (some hidden until earned), a few
// of which grant TITLES shown on the character. Evaluation runs AFTER the
// triggering action commits — an achievement bug must never roll back a
// completion — and awarding is idempotent (UNIQUE + ON CONFLICT in the store).

type achievementCtx struct {
	completions int
	doneToday   int
	streak      models.Streak
	attrs       []models.Attribute
	gold        int64
	bossKills   int
	hours       []int // local completion hours
	rarities    map[string]int
}

type achievementDef struct {
	Key    string
	Name   string
	Desc   string
	Icon   string
	Hidden bool   // invisible in the hall until earned
	Title  string // character title granted (optional)
	Check  func(c achievementCtx) bool
}

func anyAttrLevel(attrs []models.Attribute, min int) bool {
	for _, a := range attrs {
		if LevelForXP(a.TotalXP) >= min {
			return true
		}
	}
	return false
}

func allAttrLevel(attrs []models.Attribute, min int) bool {
	for _, a := range attrs {
		if LevelForXP(a.TotalXP) < min {
			return false
		}
	}
	return len(attrs) > 0
}

func anyHour(hours []int, from, to int) bool {
	for _, h := range hours {
		if h >= from && h < to {
			return true
		}
	}
	return false
}

// achievementCatalog is ordered — the hall renders in this order.
var achievementCatalog = []achievementDef{
	{Key: "first_blood", Name: "First Blood", Desc: "Complete your first quest.", Icon: "🗡️",
		Check: func(c achievementCtx) bool { return c.completions >= 1 }},
	{Key: "on_fire", Name: "On Fire", Desc: "Reach a 3-day streak.", Icon: "🔥",
		Check: func(c achievementCtx) bool { return c.streak.Current >= 3 || c.streak.Longest >= 3 }},
	{Key: "habitual", Name: "Habitual", Desc: "Reach a 7-day streak.", Icon: "📿", Title: "the Consistent",
		Check: func(c achievementCtx) bool { return c.streak.Current >= 7 || c.streak.Longest >= 7 }},
	{Key: "unbreakable", Name: "Unbreakable", Desc: "Reach a 30-day streak.", Icon: "💎", Title: "the Unbreakable",
		Check: func(c achievementCtx) bool { return c.streak.Current >= 30 || c.streak.Longest >= 30 }},
	{Key: "unstoppable", Name: "Unstoppable", Desc: "Complete 5 quests in one day.", Icon: "⚡", Title: "the Unstoppable",
		Check: func(c achievementCtx) bool { return c.doneToday >= 5 }},
	{Key: "boss_slayer", Name: "Boss Slayer", Desc: "Bring down a boss quest.", Icon: "💀", Title: "Bossbane",
		Check: func(c achievementCtx) bool { return c.bossKills >= 1 }},
	{Key: "adept", Name: "Adept", Desc: "Raise any attribute to level 5.", Icon: "🎓",
		Check: func(c achievementCtx) bool { return anyAttrLevel(c.attrs, 5) }},
	{Key: "ascendant", Name: "Ascendant", Desc: "Raise any attribute to level 10.", Icon: "🌟", Title: "the Ascendant",
		Check: func(c achievementCtx) bool { return anyAttrLevel(c.attrs, 10) }},
	{Key: "renaissance", Name: "Renaissance Soul", Desc: "Get every attribute to level 2+.", Icon: "🎭",
		Check: func(c achievementCtx) bool { return allAttrLevel(c.attrs, 2) }},
	{Key: "collector", Name: "Collector", Desc: "Hold 10 pieces of loot.", Icon: "🎒",
		Check: func(c achievementCtx) bool {
			total := 0
			for _, n := range c.rarities {
				total += n
			}
			return total >= 10
		}},
	{Key: "dragons_hoard", Name: "Dragon's Hoard", Desc: "Bank 250 gold.", Icon: "🐲",
		Check: func(c achievementCtx) bool { return c.gold >= 250 }},
	{Key: "centurion", Name: "Centurion", Desc: "Complete 100 quests.", Icon: "🏛️", Title: "the Centurion",
		Check: func(c achievementCtx) bool { return c.completions >= 100 }},
	// Hidden until earned — the fun of stumbling into them.
	{Key: "early_bird", Name: "Early Bird", Desc: "Complete a quest before 7 in the morning.", Icon: "🌅", Hidden: true,
		Check: func(c achievementCtx) bool { return anyHour(c.hours, 4, 7) }},
	{Key: "night_owl", Name: "Night Owl", Desc: "Complete a quest after 23:00.", Icon: "🦉", Hidden: true,
		Check: func(c achievementCtx) bool { return anyHour(c.hours, 23, 24) }},
	{Key: "jackpot", Name: "Jackpot", Desc: "A legendary item found YOU.", Icon: "🎰", Hidden: true, Title: "the Chosen",
		Check: func(c achievementCtx) bool { return c.rarities["legendary"] >= 1 }},
}

// gatherAchievementCtx collects the facts the catalog checks against.
func (s *Service) gatherAchievementCtx() (achievementCtx, error) {
	var c achievementCtx
	var err error
	if c.completions, err = s.store.CountCompletions(s.userID); err != nil {
		return c, err
	}
	if c.doneToday, err = s.store.CompletedTodayCount(s.userID); err != nil {
		return c, err
	}
	if c.streak, err = s.store.GetStreak(s.userID); err != nil {
		return c, err
	}
	if c.attrs, err = s.store.ListAttributes(s.userID); err != nil {
		return c, err
	}
	if c.gold, err = s.store.GoldBalance(s.userID); err != nil {
		return c, err
	}
	if c.bossKills, err = s.store.CountBossCompletions(s.userID); err != nil {
		return c, err
	}
	if c.hours, err = s.store.CompletionHours(s.userID); err != nil {
		return c, err
	}
	if c.rarities, err = s.store.CountItemsByRarity(s.userID); err != nil {
		return c, err
	}
	return c, nil
}

// evaluateAchievements awards every newly satisfied achievement and returns
// the fresh ones. Best-effort by design: on any error it logs and returns
// what it has — the triggering action already committed.
func (s *Service) evaluateAchievements() []models.Achievement {
	ctx, err := s.gatherAchievementCtx()
	if err != nil {
		log.Printf("achievements: gather (user %d): %v", s.userID, err)
		return nil
	}
	earned, err := s.store.EarnedAchievements(s.userID)
	if err != nil {
		log.Printf("achievements: earned (user %d): %v", s.userID, err)
		return nil
	}
	var fresh []models.Achievement
	for _, def := range achievementCatalog {
		if _, has := earned[def.Key]; has || !def.Check(ctx) {
			continue
		}
		first, err := s.store.AwardAchievement(s.userID, def.Key)
		if err != nil {
			log.Printf("achievements: award %s (user %d): %v", def.Key, s.userID, err)
			continue
		}
		if first {
			now := time.Now().UTC()
			fresh = append(fresh, models.Achievement{
				Key: def.Key, Name: def.Name, Desc: def.Desc, Icon: def.Icon,
				Title: def.Title, Earned: true, AwardedAt: &now,
			})
		}
	}
	return fresh
}

// ListAchievements returns the hall: every earned badge plus the visible
// unearned ones (hidden defs stay invisible until earned).
func (s *Service) ListAchievements() ([]models.Achievement, error) {
	earned, err := s.store.EarnedAchievements(s.userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.Achievement, 0, len(achievementCatalog))
	for _, def := range achievementCatalog {
		at, has := earned[def.Key]
		if def.Hidden && !has {
			continue
		}
		a := models.Achievement{Key: def.Key, Name: def.Name, Desc: def.Desc, Icon: def.Icon, Title: def.Title, Earned: has}
		if has {
			t := at
			a.AwardedAt = &t
		}
		out = append(out, a)
	}
	return out, nil
}

// characterTitle resolves the displayed title: the most recently earned
// title-bearing achievement ("" = none yet).
func (s *Service) characterTitle() string {
	earned, err := s.store.EarnedAchievements(s.userID)
	if err != nil {
		return ""
	}
	var best string
	var bestAt time.Time
	for _, def := range achievementCatalog {
		if def.Title == "" {
			continue
		}
		if at, has := earned[def.Key]; has && at.After(bestAt) {
			best, bestAt = def.Title, at
		}
	}
	return best
}
