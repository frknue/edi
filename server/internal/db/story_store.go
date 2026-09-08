package db

import (
	"time"

	"edi/internal/models"
)

// InsertStoryChapter appends the next chapter of the user's saga.
func (s *Store) InsertStoryChapter(userID int64, text string, now time.Time) (models.StoryChapter, error) {
	var ch models.StoryChapter
	err := s.db.QueryRow(
		`INSERT INTO story_chapters(user_id, number, text, created_at)
		 VALUES($1, (SELECT COALESCE(MAX(number),0)+1 FROM story_chapters WHERE user_id = $1), $2, $3)
		 RETURNING id, number, text, created_at`, userID, text, now).
		Scan(&ch.ID, &ch.Number, &ch.Text, &ch.CreatedAt)
	return ch, err
}

// ListStoryChapters returns the newest chapters first.
func (s *Store) ListStoryChapters(userID int64, limit int) ([]models.StoryChapter, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`SELECT id, number, text, created_at FROM story_chapters WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.StoryChapter
	for rows.Next() {
		var ch models.StoryChapter
		if err := rows.Scan(&ch.ID, &ch.Number, &ch.Text, &ch.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}
