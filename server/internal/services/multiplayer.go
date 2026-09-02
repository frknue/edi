package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"edi/internal/db"
	"edi/internal/models"
)

const questBoardInviteTTL = 24 * time.Hour

// MultiplayerStatus returns the user's private two-player board, or board:null
// when multiplayer has not been set up yet.
func (s *Service) MultiplayerStatus() (models.MultiplayerStatus, error) {
	board, err := s.store.QuestBoardForUser(s.userID)
	if errors.Is(err, db.ErrNotFound) {
		return models.MultiplayerStatus{}, nil
	}
	if err != nil {
		return models.MultiplayerStatus{}, err
	}
	board.Members = orEmpty(board.Members)
	return models.MultiplayerStatus{Board: &board}, nil
}

func (s *Service) CreateQuestBoard(name string) (models.QuestBoard, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Quest Party"
	}
	if len(name) > 60 {
		return models.QuestBoard{}, validationErr("board name is too long (max 60 characters)")
	}
	board, err := s.store.CreateQuestBoard(s.userID, name)
	if errors.Is(err, db.ErrAlreadyInBoard) {
		return models.QuestBoard{}, validationErr("you are already in a quest board")
	}
	return board, err
}

func (s *Service) CreateQuestBoardInvite() (models.QuestBoardInvite, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return models.QuestBoardInvite{}, err
	}
	hexCode := strings.ToUpper(hex.EncodeToString(raw))
	code := hexCode[:4] + "-" + hexCode[4:8] + "-" + hexCode[8:]
	expiresAt := time.Now().UTC().Add(questBoardInviteTTL)
	err := s.store.CreateQuestBoardInvite(s.userID, hashToken(normalizeBoardCode(code)), expiresAt)
	switch {
	case errors.Is(err, db.ErrNotFound):
		return models.QuestBoardInvite{}, validationErr("create a quest board before inviting someone")
	case errors.Is(err, db.ErrBoardFull):
		return models.QuestBoardInvite{}, validationErr("this quest board already has two members")
	case err != nil:
		return models.QuestBoardInvite{}, err
	}
	return models.QuestBoardInvite{Code: code, ExpiresAt: expiresAt}, nil
}

func normalizeBoardCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func (s *Service) JoinQuestBoard(code string) (models.QuestBoard, error) {
	code = normalizeBoardCode(code)
	if len(code) != 12 {
		return models.QuestBoard{}, validationErr("enter a valid quest board invite code")
	}
	board, err := s.store.JoinQuestBoard(s.userID, hashToken(code), time.Now().UTC())
	switch {
	case errors.Is(err, db.ErrAlreadyInBoard):
		return models.QuestBoard{}, validationErr("you are already in a quest board")
	case errors.Is(err, db.ErrBoardFull):
		return models.QuestBoard{}, validationErr("this quest board already has two members")
	case errors.Is(err, db.ErrInvalidInvite):
		return models.QuestBoard{}, validationErr("this invite code is invalid, expired, or already used")
	case err != nil:
		return models.QuestBoard{}, err
	}
	board.Members = orEmpty(board.Members)
	return board, nil
}

func (s *Service) validateSharedAssignees(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, validationErr("choose at least one quest assignee")
	}
	status, err := s.MultiplayerStatus()
	if err != nil {
		return nil, err
	}
	if status.Board == nil {
		return nil, validationErr("create or join a quest board before assigning a shared quest")
	}
	members := map[int64]bool{}
	for _, member := range status.Board.Members {
		members[member.UserID] = true
	}
	seen := map[int64]bool{}
	clean := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !members[id] {
			return nil, validationErr("assignee %d is not on your quest board", id)
		}
		if !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	return clean, nil
}
