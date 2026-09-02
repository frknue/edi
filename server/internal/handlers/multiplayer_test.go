package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edi/internal/models"
)

func TestMultiplayerHTTPFlow(t *testing.T) {
	t.Setenv("EDI_INVITE_CODE", "sesame")
	router := newTestRouter(t, "admin-token")
	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, reader)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := do(http.MethodPost, "/api/auth/register", "", `{"name":"Partner","invite_code":"sesame"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register partner = %d %s", rec.Code, rec.Body.String())
	}
	var partner models.CreatedUser
	if err := json.Unmarshal(rec.Body.Bytes(), &partner); err != nil {
		t.Fatal(err)
	}

	if rec = do(http.MethodGet, "/api/multiplayer", "admin-token", ""); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"board":null}` {
		t.Fatalf("empty multiplayer status = %d %s", rec.Code, rec.Body.String())
	}
	if rec = do(http.MethodPost, "/api/multiplayer/board", "admin-token", `{"name":"Pair"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create board = %d %s", rec.Code, rec.Body.String())
	}
	if rec = do(http.MethodPost, "/api/multiplayer/invite", "admin-token", `{}`); rec.Code != http.StatusCreated {
		t.Fatalf("create invite = %d %s", rec.Code, rec.Body.String())
	}
	var invite models.QuestBoardInvite
	if err := json.Unmarshal(rec.Body.Bytes(), &invite); err != nil {
		t.Fatal(err)
	}
	if rec = do(http.MethodPost, "/api/multiplayer/join", partner.Token, fmt.Sprintf(`{"code":%q}`, invite.Code)); rec.Code != http.StatusOK {
		t.Fatalf("join = %d %s", rec.Code, rec.Body.String())
	}

	body := fmt.Sprintf(`{"title":"HTTP pair quest","type":"side","difficulty":"easy","attribute_rewards":{"relationships":10},"assignee_ids":[1,%d]}`, partner.User.ID)
	if rec = do(http.MethodPost, "/api/quests", "admin-token", body); rec.Code != http.StatusCreated {
		t.Fatalf("create shared quest = %d %s", rec.Code, rec.Body.String())
	}
	var created models.Quest
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.SharedQuestID == nil || len(created.Assignees) != 2 {
		t.Fatalf("created shared metadata = %+v", created)
	}

	if rec = do(http.MethodGet, "/api/quests", partner.Token, ""); rec.Code != http.StatusOK {
		t.Fatalf("partner list = %d %s", rec.Code, rec.Body.String())
	}
	var partnerQuests []models.Quest
	if err := json.Unmarshal(rec.Body.Bytes(), &partnerQuests); err != nil {
		t.Fatal(err)
	}
	var partnerQuest *models.Quest
	for i := range partnerQuests {
		if partnerQuests[i].SharedQuestID != nil && *partnerQuests[i].SharedQuestID == *created.SharedQuestID {
			partnerQuest = &partnerQuests[i]
			break
		}
	}
	if partnerQuest == nil || !partnerQuest.AssignedToMe || partnerQuest.ID == created.ID {
		t.Fatalf("partner quest = %+v", partnerQuest)
	}
	if rec = do(http.MethodPost, fmt.Sprintf("/api/quests/%d/complete", partnerQuest.ID), partner.Token, ""); rec.Code != http.StatusOK {
		t.Fatalf("partner complete = %d %s", rec.Code, rec.Body.String())
	}
	var completion models.CompletionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &completion); err != nil {
		t.Fatal(err)
	}
	if completion.Quest.Status != models.StatusActive || completion.Quest.MyStatus != models.StatusCompleted || completion.Quest.AllCompleted {
		t.Errorf("partner completion card = %+v", completion.Quest)
	}
}
