package handlers

import (
	"net/http"

	"edi/internal/models"
)

func (h *Handlers) multiplayerStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.forUser(r).MultiplayerStatus()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) createQuestBoard(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	board, err := h.forUser(r).CreateQuestBoard(in.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, board)
}

func (h *Handlers) createQuestBoardInvite(w http.ResponseWriter, r *http.Request) {
	invite, err := h.forUser(r).CreateQuestBoardInvite()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func (h *Handlers) joinQuestBoard(w http.ResponseWriter, r *http.Request) {
	var in models.JoinQuestBoardInput
	if err := decodeBody(r, &in); err != nil {
		writeError(w, err)
		return
	}
	board, err := h.forUser(r).JoinQuestBoard(in.Code)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}
