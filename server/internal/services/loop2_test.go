package services

import (
	"testing"

	"edi/internal/models"
)

// The projection on every card must equal what the award path actually pays
// (combo + buffs + checked subtasks, no crit) — or trust dies.
func TestProjectionMatchesAward(t *testing.T) {
	svc := newTestService(t)
	mk := func(title string, rewards map[string]int64, subs ...models.SubtaskInput) models.Quest {
		q, err := svc.CreateQuest(models.QuestInput{Title: title, AttributeRewards: rewards, Subtasks: subs})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return q
	}
	dropper := mk("dropper", map[string]int64{"focus": 40})
	target := mk("target", map[string]int64{"focus": 37, "strength": 13}, models.SubtaskInput{Title: "bonus", AttributeRewards: map[string]int64{"focus": 9}})
	if _, err := svc.ToggleSubtask(target.ID, target.Subtasks[0].ID); err != nil {
		t.Fatal(err)
	}
	// 1st completion drops the epic prism (+25% ALL); no crit.
	svc.store.SetRollForTest(seqRoll(0.99, 0.0, 0.96, 0.0))
	if _, err := svc.CompleteQuest(dropper.ID); err != nil {
		t.Fatal(err)
	}
	svc.store.SetRollForTest(func() float64 { return 0.99 })

	dash, err := svc.GetDashboard()
	if err != nil {
		t.Fatal(err)
	}
	var projected int64
	for _, q := range dash.TodayQuests {
		if q.ID == target.ID {
			projected = q.ProjectedXP
		}
	}
	if projected == 0 {
		t.Fatal("no projection on the target card")
	}
	res, err := svc.CompleteQuest(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	var paid int64
	for _, e := range res.XPEvents {
		if e.Source != "board_clear" {
			paid += e.Amount
		}
	}
	if paid != projected {
		t.Errorf("projected %d, paid %d (combo ×%.2f, buff +25%%, subtask 9)", projected, paid, res.ComboMultiplier)
	}
	if res.Quest.RecommendReason != "" || dash.RecommendedQuest == nil || dash.RecommendedQuest.RecommendReason == "" {
		t.Errorf("recommend reason missing: %+v", dash.RecommendedQuest)
	}
}

// Clearing the daily set pays the board_clear bonus exactly once per local
// day, inside the completion tx, and flips the day into camp.
func TestBoardClearBonusOncePerDay(t *testing.T) {
	svc := newTestService(t)
	svc.store.SetRollForTest(func() float64 { return 0.99 })
	archiveSeedDailies(t, svc)
	a, _ := svc.CreateQuest(models.QuestInput{Title: "A", Type: "daily", AttributeRewards: map[string]int64{"health": 20}})
	b, _ := svc.CreateQuest(models.QuestInput{Title: "B", Type: "daily", AttributeRewards: map[string]int64{"health": 20}})
	side, _ := svc.CreateQuest(models.QuestInput{Title: "S", Type: "side", AttributeRewards: map[string]int64{"health": 20}})

	r1, _ := svc.CompleteQuest(a.ID)
	if r1.BoardClear || r1.Dashboard.DayState != "open" || r1.Dashboard.DailyProgress.DailiesDone != 1 || r1.Dashboard.DailyProgress.Goal != 2 {
		t.Fatalf("after A: clear=%v state=%s progress=%+v", r1.BoardClear, r1.Dashboard.DayState, r1.Dashboard.DailyProgress)
	}
	rs, _ := svc.CompleteQuest(side.ID)
	if rs.BoardClear || rs.Dashboard.DailyProgress.Cleared {
		t.Fatal("a side quest must not close the daily set")
	}
	r2, err := svc.CompleteQuest(b.ID)
	if err != nil || !r2.BoardClear || r2.Dashboard.DayState != "camp" || !r2.Dashboard.BoardClearToday {
		t.Fatalf("after B: %v clear=%v state=%s", err, r2.BoardClear, r2.Dashboard.DayState)
	}
	var bonus int64
	for _, e := range r2.XPEvents {
		if e.Source == "board_clear" {
			bonus += e.Amount
		}
	}
	if want := BoardClearRewards["discipline"] + BoardClearRewards["focus"]; bonus != want {
		t.Errorf("board clear bonus = %d, want %d", bonus, want)
	}
	// A later extra quest the same day pays normally, no second bonus.
	extra, _ := svc.CreateQuest(models.QuestInput{Title: "E", Type: "side", AttributeRewards: map[string]int64{"health": 20}})
	r3, _ := svc.CompleteQuest(extra.ID)
	if r3.BoardClear {
		t.Error("bonus paid twice")
	}
	if r3.Dashboard.XPToday <= 0 {
		t.Errorf("xp_today = %d", r3.Dashboard.XPToday)
	}
	if auditDrift(t, svc) != 0 {
		t.Error("audit invariant violated")
	}
}

// The pinned first move wins the recommendation on its day, is dropped when
// stale or completed, and is never a skip.
func TestFirstMove(t *testing.T) {
	svc := newTestService(t)
	quests, _ := svc.ListQuests("", "active")
	var pick models.Quest
	for _, q := range quests {
		if q.Type != "boss" && !q.AssignedToMe == false && q.SharedQuestID == nil {
			pick = q
		}
	}
	if _, err := svc.SetFirstMove(models.FirstMoveInput{QuestID: 999999}); err == nil {
		t.Error("unknown quest accepted")
	}
	fm, err := svc.SetFirstMove(models.FirstMoveInput{QuestID: pick.ID})
	if err != nil || fm.Quest.ID != pick.ID {
		t.Fatalf("set = %+v, %v", fm, err)
	}
	dash, _ := svc.GetDashboard()
	if dash.RecommendedQuest == nil || dash.RecommendedQuest.ID != pick.ID || dash.RecommendedQuest.RecommendReason != "first_move" {
		t.Fatalf("recommended = %+v, want the pinned quest", dash.RecommendedQuest)
	}
	if dash.FirstMove == nil || dash.FirstMove.Quest.ID != pick.ID {
		t.Errorf("dashboard first_move = %+v", dash.FirstMove)
	}
	// Tomorrow's pin does not steer today's recommendation but is visible.
	svc.SetFirstMove(models.FirstMoveInput{QuestID: pick.ID, Tomorrow: true})
	dash, _ = svc.GetDashboard()
	if dash.FirstMove == nil || dash.RecommendedQuest.RecommendReason == "first_move" {
		t.Errorf("tomorrow pin: first_move=%+v reason=%s", dash.FirstMove, dash.RecommendedQuest.RecommendReason)
	}
	// Completing the pinned quest clears it from the dashboard; skip count untouched.
	svc.SetFirstMove(models.FirstMoveInput{QuestID: pick.ID})
	if _, err := svc.CompleteQuest(pick.ID); err != nil {
		t.Fatal(err)
	}
	dash, _ = svc.GetDashboard()
	if dash.FirstMove != nil {
		t.Errorf("first move survives completion: %+v", dash.FirstMove)
	}
	if err := svc.ClearFirstMove(); err != nil {
		t.Fatal(err)
	}
	if q, _ := svc.store.GetQuest(1, pick.ID); q.SkipCount != 0 {
		t.Errorf("skip_count = %d, want 0", q.SkipCount)
	}
}
