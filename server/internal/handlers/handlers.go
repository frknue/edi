package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"edi/internal/agent"
	"edi/internal/models"
	"edi/internal/services"
)

// Handlers holds the dependencies shared by all HTTP handlers.
type Handlers struct {
	svc      *services.Service
	registry *agent.Registry
	sessions *agent.Sessions // in-memory chat history for /api/agent/chat
}

func New(svc *services.Service, registry *agent.Registry) *Handlers {
	return &Handlers{svc: svc, registry: registry, sessions: agent.NewSessions()}
}

func (h *Handlers) health(w http.ResponseWriter, _ *http.Request) {
	// Commit is the honest way to verify what's live: EDI_COMMIT is set by the
	// CI deploy job right before `railway up` (a tarball upload carries no git
	// metadata); RAILWAY_GIT_COMMIT_SHA is what Railway injects on its own
	// GitHub-triggered deploys.
	commit := os.Getenv("EDI_COMMIT")
	if commit == "" {
		commit = os.Getenv("RAILWAY_GIT_COMMIT_SHA")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "commit": commit})
}

// --- dashboard / attributes -------------------------------------------------

func (h *Handlers) getDashboard(w http.ResponseWriter, r *http.Request) {
	dash, err := h.forUser(r).GetDashboard()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dash)
}

func (h *Handlers) getAttributes(w http.ResponseWriter, r *http.Request) {
	attrs, err := h.forUser(r).ListAttributes()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, attrs)
}

// --- quests -----------------------------------------------------------------

func (h *Handlers) listQuests(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	quests, err := h.forUser(r).ListQuests(q.Get("type"), q.Get("status"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quests)
}

func (h *Handlers) createQuest(w http.ResponseWriter, r *http.Request) {
	var in models.QuestInput
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	quest, err := h.forUser(r).CreateQuest(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, quest)
}

func (h *Handlers) recordSpontaneousQuest(w http.ResponseWriter, r *http.Request) {
	var in models.QuestInput
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	result, err := h.forUser(r).RecordSpontaneousQuest(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// draftQuest asks the AI for a quest's type/difficulty/XP. It only proposes —
// nothing is persisted; the client applies the draft to the open form.
func (h *Handlers) draftQuest(w http.ResponseWriter, r *http.Request) {
	var in models.QuestDraftRequest
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	draft, err := h.forUser(r).DraftQuest(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *Handlers) updateQuest(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var patch models.QuestPatch
	if err := decodeBody(r, &patch); err != nil {
		writeError(w, err)
		return
	}
	quest, err := h.forUser(r).UpdateQuest(id, patch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quest)
}

func (h *Handlers) completeQuest(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := h.forUser(r).CompleteQuest(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) skipQuest(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	quest, err := h.forUser(r).SkipQuest(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quest)
}

func (h *Handlers) toggleSubtask(w http.ResponseWriter, r *http.Request) {
	questID, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	subtaskID, err := pathID(r, "sid")
	if err != nil {
		writeError(w, err)
		return
	}
	st, err := h.forUser(r).ToggleSubtask(questID, subtaskID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handlers) archiveQuest(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	quest, err := h.forUser(r).ArchiveQuest(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quest)
}

// --- xp / journal -----------------------------------------------------------

func (h *Handlers) getXPEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.forUser(r).ListXPEvents(queryInt(r, "limit", 50))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handlers) listJournal(w http.ResponseWriter, r *http.Request) {
	entries, err := h.forUser(r).ListJournalEntries(queryInt(r, "limit", 30), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handlers) createJournal(w http.ResponseWriter, r *http.Request) {
	var in models.JournalInput
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	result, err := h.forUser(r).CreateJournalEntry(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handlers) updateJournal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var patch models.JournalPatch
	if err := decodeBody(r, &patch); err != nil {
		writeError(w, err)
		return
	}
	entry, err := h.forUser(r).UpdateJournalEntry(id, patch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handlers) deleteJournal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.forUser(r).DeleteJournalEntry(id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --- agent suggestions ------------------------------------------------------

func (h *Handlers) listSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := h.forUser(r).ListSuggestions(r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

func (h *Handlers) generateSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := h.forUser(r).GenerateSuggestions()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

func (h *Handlers) acceptSuggestion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	quest, err := h.forUser(r).AcceptSuggestion(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, quest)
}

func (h *Handlers) dismissSuggestion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	sug, err := h.forUser(r).DismissSuggestion(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sug)
}

// --- loot ---------------------------------------------------------------------

func (h *Handlers) listItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.forUser(r).ListItems()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) listAchievements(w http.ResponseWriter, r *http.Request) {
	out, err := h.forUser(r).ListAchievements()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// --- story mode ---------------------------------------------------------------

func (h *Handlers) storyNarration(w http.ResponseWriter, r *http.Request) {
	story, err := h.forUser(r).StoryNarration()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"story": story})
}

func (h *Handlers) forgeBoss(w http.ResponseWriter, r *http.Request) {
	quest, err := h.forUser(r).ForgeBoss()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, quest)
}

// --- tools ------------------------------------------------------------------

func (h *Handlers) listGuidedTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tools": h.forUser(r).ListTools()})
}

func (h *Handlers) completeTool(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var payload json.RawMessage
	if err := decodeBody(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	result, err := h.forUser(r).CompleteTool(key, payload)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) listToolEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := h.forUser(r).ListToolEntries(r.PathValue("key"), queryInt(r, "limit", 30))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handlers) toolAssist(w http.ResponseWriter, r *http.Request) {
	var payload json.RawMessage
	if err := decodeBody(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	result, err := h.forUser(r).ToolAssist(r.PathValue("key"), payload)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- openai connection ------------------------------------------------------

func (h *Handlers) openaiStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.forUser(r).OpenAIStatus()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) openaiConnect(w http.ResponseWriter, r *http.Request) {
	authURL, err := h.forUser(r).StartOpenAIConnect()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

// openaiConnectComplete is the remote-server path: the user pastes the
// localhost:1455 URL their browser landed on after signing in.
func (h *Handlers) openaiConnectComplete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CallbackURL string `json:"callback_url"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	status, err := h.forUser(r).CompleteOpenAIConnect(in.CallbackURL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) openaiImportCodex(w http.ResponseWriter, r *http.Request) {
	status, err := h.forUser(r).ImportCodexCredentials()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) openaiDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := h.forUser(r).DisconnectOpenAI(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"connected": false})
}

func (h *Handlers) openaiModels(w http.ResponseWriter, r *http.Request) {
	list, err := h.forUser(r).ListOpenAIModels()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": list})
}

func (h *Handlers) openaiConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model  string `json:"model"`
		Effort string `json:"effort"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	status, err := h.forUser(r).SetOpenAIConfig(body.Model, body.Effort)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// --- gold economy / reward shop ----------------------------------------------

func (h *Handlers) listShop(w http.ResponseWriter, r *http.Request) {
	items, err := h.forUser(r).ListShopItems()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) createShopItem(w http.ResponseWriter, r *http.Request) {
	var in models.ShopItemInput
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	item, err := h.forUser(r).CreateShopItem(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handlers) updateShopItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var patch models.ShopItemPatch
	if err := decodeBody(r, &patch); err != nil {
		writeError(w, err)
		return
	}
	item, err := h.forUser(r).UpdateShopItem(id, patch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handlers) archiveShopItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.forUser(r).ArchiveShopItem(id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"archived": true})
}

func (h *Handlers) purchaseShopItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := h.forUser(r).PurchaseShopItem(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) listGoldEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.forUser(r).ListGoldEvents(queryInt(r, "limit", 30), r.URL.Query().Get("source"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// --- agent tool interface ---------------------------------------------------

// listTools exposes the agent-ready tool catalog (names, descriptions, schemas).
func (h *Handlers) listTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tools": h.registry.Specs()})
}

// invokeTool runs a named tool with a raw JSON input body — the exact path a
// future LLM agent uses, hitting the same services as the REST API.
func (h *Handlers) invokeTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var input json.RawMessage
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			// Malformed body -> clean 400 instead of a confusing downstream error.
			writeError(w, errors.Join(services.ErrValidation, err))
			return
		}
	}
	result, err := h.registry.Invoke(h.forUser(r), name, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tool": name, "result": result})
}

// chat is the conversational agent over HTTP: one free-text message in, the
// model's reply (after acting through the tool registry) out. History is
// per user + optional session name, in memory; reset=true clears it first.
func (h *Handlers) chat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
		Session string `json:"session"`
		Reset   bool   `json:"reset"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	svc := h.forUser(r)
	key := fmt.Sprintf("http:%d:%s", svc.UserID(), body.Session)
	if body.Reset {
		h.sessions.Reset(key)
	}
	res, err := h.registry.Chat(svc, svc.OpenAIConverse, h.sessions, key, body.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// --- decay & stakes -----------------------------------------------------------

func (h *Handlers) wardAttribute(w http.ResponseWriter, r *http.Request) {
	result, err := h.forUser(r).WardAttribute(r.PathValue("key"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) setRest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	state, err := h.forUser(r).SetRestMode(body.On)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) getRest(w http.ResponseWriter, r *http.Request) {
	state, err := h.forUser(r).RestState()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
