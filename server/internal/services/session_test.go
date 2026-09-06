package services

import (
	"errors"
	"testing"
	"time"

	"edi/internal/models"
)

// One running session per user; starting another switches; Stop stores the
// landing note on the quest; Complete closes the session and clears the note
// in the same tx; nothing ever writes XP.
func TestQuestSessionLifecycle(t *testing.T) {
	svc := newTestService(t)
	quests, _ := svc.ListQuests("", "active")
	q1, q2 := quests[0], quests[1]
	xpBefore, _ := svc.ListXPEvents(500)

	if sess, err := svc.ActiveSession(); err != nil || sess != nil {
		t.Fatalf("initial session = %+v, %v; want none", sess, err)
	}
	if _, err := svc.StopQuest(models.StopSessionInput{Note: "x"}); !errors.Is(err, ErrValidation) {
		t.Errorf("stop with nothing running = %v, want ErrValidation", err)
	}

	s1, err := svc.StartQuest(q1.ID)
	if err != nil || !s1.Running || s1.QuestID != q1.ID || s1.Title != q1.Title {
		t.Fatalf("start = %+v, %v", s1, err)
	}
	// Idempotent: starting the same quest returns the same session.
	if again, err := svc.StartQuest(q1.ID); err != nil || again.ID != s1.ID {
		t.Errorf("restart same quest = %+v, %v; want session %d", again, err, s1.ID)
	}
	dash, _ := svc.GetDashboard()
	if dash.ActiveSession == nil || dash.ActiveSession.QuestID != q1.ID {
		t.Fatalf("dashboard active_session = %+v, want quest %d", dash.ActiveSession, q1.ID)
	}

	// Switching: start q2 closes s1 as 'switched'.
	s2, err := svc.StartQuest(q2.ID)
	if err != nil || s2.ID == s1.ID {
		t.Fatalf("switch = %+v, %v", s2, err)
	}
	hist, _ := svc.ListQuestSessions(10)
	if len(hist) != 2 || hist[1].Reason != "switched" || hist[1].EndedAt == nil || hist[0].Running != true {
		t.Fatalf("history after switch = %+v", hist)
	}

	// Stop with the landing note: note lands on the quest.
	stopped, err := svc.StopQuest(models.StopSessionInput{Note: "  open the editor at main.go  "})
	if err != nil || stopped.Reason != "stopped" || stopped.Note != "open the editor at main.go" {
		t.Fatalf("stop = %+v, %v", stopped, err)
	}
	if got, _ := svc.store.GetQuest(1, q2.ID); got.ResumeNote != "open the editor at main.go" {
		t.Errorf("resume note on quest = %q", got.ResumeNote)
	}
	if sess, _ := svc.ActiveSession(); sess != nil {
		t.Errorf("session still active after stop: %+v", sess)
	}
	// Resuming shows the note; completing clears it and closes the session.
	s3, err := svc.StartQuest(q2.ID)
	if err != nil || s3.ResumeNote != "open the editor at main.go" {
		t.Fatalf("resume = %+v, %v", s3, err)
	}
	res, err := svc.CompleteQuest(q2.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if res.Dashboard.ActiveSession != nil || res.Quest.ResumeNote != "" {
		t.Errorf("after complete: session=%+v note=%q; want closed + cleared", res.Dashboard.ActiveSession, res.Quest.ResumeNote)
	}
	hist, _ = svc.ListQuestSessions(10)
	if hist[0].Reason != "completed" {
		t.Errorf("last session reason = %q, want completed", hist[0].Reason)
	}
	// Sessions never wrote XP: only the completion did.
	xpAfter, _ := svc.ListXPEvents(500)
	for _, e := range xpAfter[:len(xpAfter)-len(xpBefore)] {
		if e.Source != "quest" && e.Source != "combo" && e.Source != "crit" && e.Source != "buff" {
			t.Errorf("unexpected xp source %q from a session", e.Source)
		}
	}
	// Starting a completed quest is a client error.
	if _, err := svc.StartQuest(q2.ID); !errors.Is(err, ErrValidation) {
		t.Errorf("start completed quest = %v, want ErrValidation", err)
	}
	if _, err := svc.StartQuest(999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("start unknown quest = %v, want ErrNotFound", err)
	}
	long := make([]byte, maxResumeNote+1)
	for i := range long {
		long[i] = 'a'
	}
	svc.StartQuest(q1.ID)
	if _, err := svc.StopQuest(models.StopSessionInput{Note: string(long)}); !errors.Is(err, ErrValidation) {
		t.Errorf("overlong note = %v, want ErrValidation", err)
	}
	if auditDrift(t, svc) != 0 {
		t.Error("audit invariant violated")
	}
}

// A session left running from a previous local day expires on read, so a
// stale timer never greets the user in the morning.
func TestQuestSessionExpiresAcrossDays(t *testing.T) {
	svc := newTestService(t)
	quests, _ := svc.ListQuests("", "active")
	sess, err := svc.StartQuest(quests[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().AddDate(0, 0, -1)
	if _, err := svc.store.DB().Exec(`UPDATE quest_sessions SET started_at = $1 WHERE id = $2`, yesterday, sess.ID); err != nil {
		t.Fatal(err)
	}
	if active, err := svc.ActiveSession(); err != nil || active != nil {
		t.Fatalf("stale session still active: %+v, %v", active, err)
	}
	hist, _ := svc.ListQuestSessions(5)
	if len(hist) != 1 || hist[0].Reason != "expired" {
		t.Errorf("expired session = %+v", hist)
	}
}
