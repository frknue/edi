package agent

import (
	"encoding/json"
	"fmt"
	"testing"

	"edi/internal/db/dbtest"
	"edi/internal/models"
	"edi/internal/services"
)

func TestAgentCanInspectBoardAndCreateSharedQuest(t *testing.T) {
	store := dbtest.Open(t)
	if err := store.Seed(); err != nil {
		t.Fatal(err)
	}
	base := services.New(store, 1)
	partner, err := base.CreateUser("Partner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base.CreateQuestBoard("Pair"); err != nil {
		t.Fatal(err)
	}
	invite, err := base.CreateQuestBoardInvite()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base.ForUser(partner.User.ID).JoinQuestBoard(invite.Code); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	got, err := registry.Invoke(base, "get_quest_board", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_quest_board: %v", err)
	}
	status, ok := got.(models.MultiplayerStatus)
	if !ok || status.Board == nil || len(status.Board.Members) != 2 {
		t.Fatalf("board tool result = %#v", got)
	}

	input := json.RawMessage(fmt.Sprintf(
		`{"title":"Agent pair quest","type":"side","difficulty":"easy","attribute_rewards":{"relationships":10},"assignee_ids":[1,%d]}`,
		partner.User.ID))
	got, err = registry.Invoke(base, "create_quest", input)
	if err != nil {
		t.Fatalf("create_quest: %v", err)
	}
	quest, ok := got.(models.Quest)
	if !ok || quest.SharedQuestID == nil || len(quest.Assignees) != 2 {
		t.Fatalf("shared quest tool result = %#v", got)
	}
}
