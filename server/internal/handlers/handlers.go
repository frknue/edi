package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"edi/internal/agent"
	"edi/internal/models"
	"edi/internal/services"
)

// Handlers holds the dependencies shared by all HTTP handlers.
type Handlers struct {
	svc      *services.Service
	registry *agent.Registry
}

func New(svc *services.Service, registry *agent.Registry) *Handlers {
	return &Handlers{svc: svc, registry: registry}
}

func (h *Handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- dashboard / attributes -------------------------------------------------

func (h *Handlers) getDashboard(w http.ResponseWriter, _ *http.Request) {
	dash, err := h.svc.GetDashboard()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dash)
}

func (h *Handlers) getAttributes(w http.ResponseWriter, _ *http.Request) {
	attrs, err := h.svc.ListAttributes()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, attrs)
}

// --- quests -----------------------------------------------------------------

func (h *Handlers) listQuests(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	quests, err := h.svc.ListQuests(q.Get("type"), q.Get("status"))
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
	quest, err := h.svc.CreateQuest(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, quest)
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
	quest, err := h.svc.UpdateQuest(id, patch)
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
	result, err := h.svc.CompleteQuest(id)
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
	quest, err := h.svc.SkipQuest(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quest)
}

func (h *Handlers) archiveQuest(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	quest, err := h.svc.ArchiveQuest(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quest)
}

// --- xp / journal -----------------------------------------------------------

func (h *Handlers) getXPEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.svc.ListXPEvents(queryInt(r, "limit", 50))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handlers) listJournal(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.ListJournalEntries(queryInt(r, "limit", 30))
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
	entry, err := h.svc.CreateJournalEntry(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// --- agent suggestions ------------------------------------------------------

func (h *Handlers) listSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := h.svc.ListSuggestions(r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

func (h *Handlers) generateSuggestions(w http.ResponseWriter, _ *http.Request) {
	suggestions, err := h.svc.GenerateSuggestions()
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
	quest, err := h.svc.AcceptSuggestion(id)
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
	sug, err := h.svc.DismissSuggestion(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sug)
}

// --- openai connection ------------------------------------------------------

func (h *Handlers) openaiStatus(w http.ResponseWriter, _ *http.Request) {
	status, err := h.svc.OpenAIStatus()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) openaiConnect(w http.ResponseWriter, _ *http.Request) {
	authURL, err := h.svc.StartOpenAIConnect()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

func (h *Handlers) openaiImportCodex(w http.ResponseWriter, _ *http.Request) {
	status, err := h.svc.ImportCodexCredentials()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) openaiDisconnect(w http.ResponseWriter, _ *http.Request) {
	if err := h.svc.DisconnectOpenAI(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"connected": false})
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
	status, err := h.svc.SetOpenAIConfig(body.Model, body.Effort)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// --- agent tool interface ---------------------------------------------------

// listTools exposes the agent-ready tool catalog (names, descriptions, schemas).
func (h *Handlers) listTools(w http.ResponseWriter, _ *http.Request) {
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
	result, err := h.registry.Invoke(name, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tool": name, "result": result})
}
