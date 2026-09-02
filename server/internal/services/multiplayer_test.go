package services

import (
	"errors"
	"sync"
	"testing"

	"edi/internal/models"
)

func TestSharedQuestBoardIndependentCompletionAndAudit(t *testing.T) {
	base := newTestService(t)
	base.store.SetRollForTest(func() float64 { return 1 }) // no critical-hit bonus

	createdA, err := base.CreateUser("Ada")
	if err != nil {
		t.Fatalf("create Ada: %v", err)
	}
	createdB, err := base.CreateUser("Ben")
	if err != nil {
		t.Fatalf("create Ben: %v", err)
	}
	createdC, err := base.CreateUser("Cy")
	if err != nil {
		t.Fatalf("create Cy: %v", err)
	}
	a := base.ForUser(createdA.User.ID)
	b := base.ForUser(createdB.User.ID)
	c := base.ForUser(createdC.User.ID)

	if _, err := a.CreateQuestBoard("Ada + Ben"); err != nil {
		t.Fatalf("create board: %v", err)
	}
	invite, err := a.CreateQuestBoardInvite()
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	board, err := b.JoinQuestBoard(invite.Code)
	if err != nil {
		t.Fatalf("join board: %v", err)
	}
	if len(board.Members) != 2 {
		t.Fatalf("board members = %d, want 2", len(board.Members))
	}
	if _, err := c.JoinQuestBoard(invite.Code); !errors.Is(err, ErrValidation) {
		t.Fatalf("reuse invite = %v, want ErrValidation", err)
	}
	if _, err := a.CreateQuestBoardInvite(); !errors.Is(err, ErrValidation) {
		t.Fatalf("invite on full board = %v, want ErrValidation", err)
	}

	shared, err := a.CreateQuest(models.QuestInput{
		Title:            "Cook dinner together",
		Type:             models.QuestTypeSide,
		Difficulty:       "easy",
		AttributeRewards: map[string]int64{"relationships": 20},
		Subtasks: []models.SubtaskInput{{
			Title: "Clear the table", AttributeRewards: map[string]int64{"discipline": 5},
		}},
		AssigneeIDs: []int64{createdA.User.ID, createdB.User.ID},
	})
	if err != nil {
		t.Fatalf("create shared quest: %v", err)
	}
	if shared.SharedQuestID == nil || len(shared.Assignees) != 2 || !shared.AssignedToMe {
		t.Fatalf("shared quest metadata = %+v", shared)
	}

	questsB, err := b.ListQuests("", models.StatusActive)
	if err != nil {
		t.Fatalf("Ben list: %v", err)
	}
	sharedB := questBySharedID(t, questsB, *shared.SharedQuestID)
	if sharedB.ID == shared.ID {
		t.Fatal("both players received the same physical quest id; want per-assignee copies")
	}
	if _, err := b.ToggleSubtask(sharedB.ID, sharedB.Subtasks[0].ID); err != nil {
		t.Fatalf("Ben toggle own subtask: %v", err)
	}

	resultB, err := b.CompleteQuest(sharedB.ID)
	if err != nil {
		t.Fatalf("Ben complete: %v", err)
	}
	if resultB.Quest.Status != models.StatusActive || resultB.Quest.MyStatus != models.StatusCompleted || resultB.Quest.AllCompleted {
		t.Errorf("after Ben completes: status=%q my=%q all=%v, want active/completed/false",
			resultB.Quest.Status, resultB.Quest.MyStatus, resultB.Quest.AllCompleted)
	}
	if got := attrByKey(resultB.Dashboard.Attributes, "relationships").TotalXP; got != 20 {
		t.Errorf("Ben relationships = %d, want 20", got)
	}
	if got := attrByKey(resultB.Dashboard.Attributes, "discipline").TotalXP; got != 5 {
		t.Errorf("Ben subtask discipline = %d, want 5", got)
	}
	attrsA, _ := a.ListAttributes()
	if got := attrByKey(attrsA, "relationships").TotalXP; got != 0 {
		t.Errorf("Ada received Ben's XP: %d", got)
	}
	if _, err := b.CompleteQuest(sharedB.ID); !errors.Is(err, ErrValidation) {
		t.Errorf("Ben double complete = %v, want ErrValidation", err)
	}
	if _, err := a.UpdateQuest(shared.ID, models.QuestPatch{Title: ptr("Changed after completion")}); !errors.Is(err, ErrValidation) {
		t.Errorf("edit after one completion = %v, want ErrValidation", err)
	}

	resultA, err := a.CompleteQuest(shared.ID)
	if err != nil {
		t.Fatalf("Ada complete: %v", err)
	}
	if resultA.Quest.Status != models.StatusCompleted || resultA.Quest.MyStatus != models.StatusCompleted || !resultA.Quest.AllCompleted {
		t.Errorf("after both complete: status=%q my=%q all=%v, want completed/completed/true",
			resultA.Quest.Status, resultA.Quest.MyStatus, resultA.Quest.AllCompleted)
	}
	attrsA = resultA.Dashboard.Attributes
	if got := attrByKey(attrsA, "relationships").TotalXP; got != 20 {
		t.Errorf("Ada relationships = %d, want 20", got)
	}

	// Both character ledgers independently retain the core audit invariant.
	for _, userID := range []int64{createdA.User.ID, createdB.User.ID} {
		rows, err := base.store.DB().Query(
			`SELECT a.key, a.total_xp, COALESCE(SUM(e.amount), 0)
			 FROM attributes a LEFT JOIN xp_events e
			 ON e.user_id = a.user_id AND e.attribute_key = a.key
			 WHERE a.user_id = $1 GROUP BY a.key, a.total_xp`, userID)
		if err != nil {
			t.Fatalf("audit query: %v", err)
		}
		for rows.Next() {
			var key string
			var total, ledger int64
			if err := rows.Scan(&key, &total, &ledger); err != nil {
				rows.Close()
				t.Fatalf("audit scan: %v", err)
			}
			if total != ledger {
				t.Errorf("user %d %s total=%d ledger=%d", userID, key, total, ledger)
			}
		}
		rows.Close()
	}

	if quests, _ := c.ListQuests("", ""); len(quests) != 0 {
		t.Errorf("outsider sees %d shared quests, want 0", len(quests))
	}
	if _, err := c.CompleteQuest(shared.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("outsider complete = %v, want ErrNotFound", err)
	}
}

func TestSharedQuestCanTargetOneBoardMember(t *testing.T) {
	base := newTestService(t)
	base.store.SetRollForTest(func() float64 { return 1 })
	createdA, _ := base.CreateUser("Ada")
	createdB, _ := base.CreateUser("Ben")
	a := base.ForUser(createdA.User.ID)
	b := base.ForUser(createdB.User.ID)
	if _, err := a.CreateQuestBoard("Pair"); err != nil {
		t.Fatal(err)
	}
	invite, _ := a.CreateQuestBoardInvite()
	if _, err := b.JoinQuestBoard(invite.Code); err != nil {
		t.Fatal(err)
	}

	quest, err := a.CreateQuest(models.QuestInput{
		Title: "Ben calls the dentist", Type: "side", Difficulty: "easy",
		AttributeRewards: map[string]int64{"health": 15},
		AssigneeIDs:      []int64{createdB.User.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if quest.AssignedToMe || len(quest.Assignees) != 1 || quest.Assignees[0].UserID != createdB.User.ID {
		t.Fatalf("creator's card assignment = %+v", quest)
	}
	if _, err := a.CompleteQuest(quest.ID); !errors.Is(err, ErrValidation) {
		t.Errorf("unassigned creator complete = %v, want ErrValidation", err)
	}
	newTitle := "Ben books the dentist"
	updated, err := a.UpdateQuest(quest.ID, models.QuestPatch{Title: &newTitle})
	if err != nil {
		t.Fatalf("board member edit: %v", err)
	}
	if updated.Title != newTitle {
		t.Errorf("updated title = %q", updated.Title)
	}
	questsB, _ := b.ListQuests("", "")
	forBen := questBySharedID(t, questsB, *quest.SharedQuestID)
	if forBen.Title != newTitle || !forBen.AssignedToMe {
		t.Errorf("assignee view = %+v", forBen)
	}
	if _, err := b.CompleteQuest(forBen.ID); err != nil {
		t.Fatalf("assignee complete: %v", err)
	}
}

func TestSharedQuestConcurrentPlayersCompleteOnceEach(t *testing.T) {
	base := newTestService(t)
	base.store.SetRollForTest(func() float64 { return 1 })
	createdA, _ := base.CreateUser("Ada")
	createdB, _ := base.CreateUser("Ben")
	a := base.ForUser(createdA.User.ID)
	b := base.ForUser(createdB.User.ID)
	if _, err := a.CreateQuestBoard("Pair"); err != nil {
		t.Fatal(err)
	}
	invite, _ := a.CreateQuestBoardInvite()
	if _, err := b.JoinQuestBoard(invite.Code); err != nil {
		t.Fatal(err)
	}
	questA, err := a.CreateQuest(models.QuestInput{
		Title: "Concurrent pair quest", Type: "side", Difficulty: "easy",
		AttributeRewards: map[string]int64{"relationships": 10},
		AssigneeIDs:      []int64{createdA.User.ID, createdB.User.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	questsB, _ := b.ListQuests("", "")
	questB := questBySharedID(t, questsB, *questA.SharedQuestID)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, completion := range []func() error{
		func() error { _, err := a.CompleteQuest(questA.ID); return err },
		func() error { _, err := b.CompleteQuest(questB.ID); return err },
	} {
		wg.Add(1)
		go func(fn func() error) {
			defer wg.Done()
			<-start
			errs <- fn()
		}(completion)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent completion: %v", err)
		}
	}

	final, err := a.store.GetVisibleQuest(a.userID, questA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !final.AllCompleted || final.Status != models.StatusCompleted {
		t.Errorf("final shared state = %+v", final)
	}
	for _, svc := range []*Service{a, b} {
		attrs, _ := svc.ListAttributes()
		if got := attrByKey(attrs, "relationships").TotalXP; got != 10 {
			t.Errorf("user %d relationships = %d, want exactly 10", svc.userID, got)
		}
	}
}

func questBySharedID(t *testing.T, quests []models.Quest, sharedID int64) models.Quest {
	t.Helper()
	for _, quest := range quests {
		if quest.SharedQuestID != nil && *quest.SharedQuestID == sharedID {
			return quest
		}
	}
	t.Fatalf("shared quest %d not found in %+v", sharedID, quests)
	return models.Quest{}
}

func ptr[T any](value T) *T { return &value }
