package services

import (
	"errors"
	"testing"

	"edi/internal/db/dbtest"
	"edi/internal/models"
)

func TestRegisterUserRequiresInviteCode(t *testing.T) {
	svc := newTestService(t)

	// No EDI_INVITE_CODE -> registration disabled.
	t.Setenv("EDI_INVITE_CODE", "")
	if _, err := svc.RegisterUser(models.RegisterInput{Name: "Ada", InviteCode: "whatever"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("register with registration disabled = %v, want ErrValidation", err)
	}

	t.Setenv("EDI_INVITE_CODE", "sesame")
	if _, err := svc.RegisterUser(models.RegisterInput{Name: "Ada", InviteCode: "wrong"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("register with wrong code = %v, want ErrValidation", err)
	}
	if _, err := svc.RegisterUser(models.RegisterInput{Name: "  ", InviteCode: "sesame"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("register without a name = %v, want ErrValidation", err)
	}

	created, err := svc.RegisterUser(models.RegisterInput{Name: "Ada", InviteCode: "sesame"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if created.Token == "" || len(created.Token) != 48 {
		t.Errorf("token = %q, want a 48-hex-char token", created.Token)
	}
	if created.User.IsAdmin {
		t.Error("registered users must not be admins")
	}

	// The token authenticates as the new user.
	id, err := svc.AuthenticateToken(created.Token)
	if err != nil || id != created.User.ID {
		t.Fatalf("AuthenticateToken = (%d, %v), want (%d, nil)", id, err, created.User.ID)
	}

	// A fresh character: level-1 attributes, nothing else.
	them := svc.ForUser(created.User.ID)
	attrs, err := them.ListAttributes()
	if err != nil {
		t.Fatalf("list attributes: %v", err)
	}
	if len(attrs) != 9 {
		t.Fatalf("new user has %d attributes, want 9", len(attrs))
	}
	for _, a := range attrs {
		if a.TotalXP != 0 || a.Level != 1 {
			t.Errorf("attribute %s = %d XP Lv%d, want 0 XP Lv1", a.Key, a.TotalXP, a.Level)
		}
	}
}

func TestAuthenticateTokenRejectsUnknown(t *testing.T) {
	svc := newTestService(t)
	for _, tok := range []string{"", "   ", "deadbeef"} {
		if _, err := svc.AuthenticateToken(tok); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("AuthenticateToken(%q) = %v, want ErrUnauthorized", tok, err)
		}
	}
}

// AdoptEnvToken must be idempotent (runs every boot), create user 1 on an
// empty database, and rebind the env token even after it changed.
func TestAdoptEnvToken(t *testing.T) {
	store := dbtest.Open(t) // empty: no Seed
	svc := New(store, 1)

	if err := svc.AdoptEnvToken("first-token"); err != nil {
		t.Fatalf("adopt on empty db: %v", err)
	}
	id, err := svc.AuthenticateToken("first-token")
	if err != nil || id != 1 {
		t.Fatalf("auth after adopt = (%d, %v), want (1, nil)", id, err)
	}
	u, err := svc.Me()
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if !u.IsAdmin {
		t.Error("bootstrap user 1 must be an admin")
	}

	// Re-adopt with a NEW env value (rotation via env + restart).
	if err := svc.AdoptEnvToken("second-token"); err != nil {
		t.Fatalf("re-adopt: %v", err)
	}
	if _, err := svc.AuthenticateToken("first-token"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("old token still valid after rotation: %v", err)
	}
	if id, err := svc.AuthenticateToken("second-token"); err != nil || id != 1 {
		t.Errorf("new token = (%d, %v), want (1, nil)", id, err)
	}

	// User 1's token is env-owned: the admin rotation API refuses it.
	if _, err := svc.RotateUserToken(1); !errors.Is(err, ErrValidation) {
		t.Errorf("rotate user 1 = %v, want ErrValidation", err)
	}
}

// The multi-tenant contract: one user can NEVER see or touch another user's
// data. This is the test that catches any store query missing its user_id
// filter.
func TestTenantIsolation(t *testing.T) {
	svc := newTestService(t) // seeds demo user 1
	t.Setenv("EDI_INVITE_CODE", "sesame")

	created, err := svc.RegisterUser(models.RegisterInput{Name: "Blank", InviteCode: "sesame"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	a := svc.ForUser(created.User.ID) // blank second user
	b := svc                          // seeded user 1

	// B builds up state of every kind.
	bQuest, err := b.CreateQuest(models.QuestInput{
		Title: "B's secret quest", Type: "daily", Difficulty: "easy",
		AttributeRewards: map[string]int64{"strength": 30},
		Subtasks:         []models.SubtaskInput{{Title: "B sub", AttributeRewards: map[string]int64{"focus": 5}}},
	})
	if err != nil {
		t.Fatalf("b quest: %v", err)
	}
	if _, err := b.CompleteQuest(bQuest.ID); err != nil {
		t.Fatalf("b complete: %v", err)
	}
	if _, err := b.CreateJournalEntry(models.JournalInput{Mood: 7, Energy: 7, Notes: "b private note"}); err != nil {
		t.Fatalf("b journal: %v", err)
	}
	bItem, err := b.CreateShopItem(models.ShopItemInput{Name: "B reward", Price: 5})
	if err != nil {
		t.Fatalf("b shop: %v", err)
	}

	// A sees NONE of it.
	if quests, _ := a.ListQuests("", ""); len(quests) != 0 {
		t.Errorf("A sees %d of B's quests, want 0", len(quests))
	}
	if entries, _ := a.ListJournalEntries(100, ""); len(entries) != 0 {
		t.Errorf("A sees %d of B's journal entries, want 0", len(entries))
	}
	if events, _ := a.ListXPEvents(100); len(events) != 0 {
		t.Errorf("A sees %d of B's xp events, want 0", len(events))
	}
	if items, _ := a.ListShopItems(); len(items) != 0 {
		t.Errorf("A sees %d of B's shop items, want 0", len(items))
	}
	if gold, _ := a.GoldBalance(); gold != 0 {
		t.Errorf("A's gold = %d, want 0 (B's balance must not leak)", gold)
	}
	dash, err := a.GetDashboard()
	if err != nil {
		t.Fatalf("a dashboard: %v", err)
	}
	if dash.Character.TotalXP != 0 || dash.Streak.Current != 0 || len(dash.TodayQuests) != 0 {
		t.Errorf("A's dashboard leaks B's state: xp=%d streak=%d quests=%d",
			dash.Character.TotalXP, dash.Streak.Current, len(dash.TodayQuests))
	}

	// A cannot act on B's rows by id: everything 404s.
	if _, err := a.CompleteQuest(bQuest.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("A completing B's quest = %v, want ErrNotFound", err)
	}
	if _, err := a.UpdateQuest(bQuest.ID, models.QuestPatch{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("A patching B's quest = %v, want ErrNotFound", err)
	}
	if _, err := a.PurchaseShopItem(bItem.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("A purchasing B's item = %v, want ErrNotFound", err)
	}

	// And B's own view is intact after A's probing.
	bQuests, _ := b.ListQuests("", "")
	if len(bQuests) == 0 {
		t.Error("B's quests vanished")
	}
	if entries, _ := b.ListJournalEntries(100, ""); len(entries) != 2 { // seed + new
		t.Errorf("B has %d journal entries, want 2", len(entries))
	}

	// A's actions award XP to A only.
	aQuest, err := a.CreateQuest(models.QuestInput{
		Title: "A's quest", Type: "daily", Difficulty: "easy",
		AttributeRewards: map[string]int64{"health": 20},
	})
	if err != nil {
		t.Fatalf("a quest: %v", err)
	}
	if _, err := a.CompleteQuest(aQuest.ID); err != nil {
		t.Fatalf("a complete: %v", err)
	}
	aAttrs, _ := a.ListAttributes()
	bAttrs, _ := b.ListAttributes()
	if got := attrByKey(aAttrs, "health").TotalXP; got != 20 {
		t.Errorf("A health = %d, want 20", got)
	}
	if got := attrByKey(bAttrs, "health").TotalXP; got != 90 { // seed value, untouched
		t.Errorf("B health = %d, want the seeded 90 (A's completion must not leak)", got)
	}
}
