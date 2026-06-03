package services

import "testing"

func TestLevelForXP(t *testing.T) {
	cases := []struct {
		xp   int64
		want int
	}{
		{0, 1},
		{99, 1},
		{100, 2}, // sqrt(1)=1 -> level 2
		{399, 2},
		{400, 3},  // sqrt(4)=2 -> level 3
		{900, 4},  // sqrt(9)=3 -> level 4
		{2500, 6}, // sqrt(25)=5 -> level 6
		{-50, 1},  // negative clamps to 0
	}
	for _, c := range cases {
		if got := LevelForXP(c.xp); got != c.want {
			t.Errorf("LevelForXP(%d) = %d, want %d", c.xp, got, c.want)
		}
	}
}

func TestXPForLevel(t *testing.T) {
	cases := []struct {
		level int
		want  int64
	}{
		{1, 0},
		{2, 100},
		{3, 400},
		{4, 900},
		{6, 2500},
	}
	for _, c := range cases {
		if got := XPForLevel(c.level); got != c.want {
			t.Errorf("XPForLevel(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestProgressForXP(t *testing.T) {
	// 150 XP: level 2 (starts at 100), next level at 400 -> 50/300 of the way.
	level, into, forNext, ratio := ProgressForXP(150)
	if level != 2 {
		t.Fatalf("level = %d, want 2", level)
	}
	if into != 50 {
		t.Errorf("xpIntoLevel = %d, want 50", into)
	}
	if forNext != 300 {
		t.Errorf("xpForNextLevel = %d, want 300", forNext)
	}
	if ratio < 0.16 || ratio > 0.17 {
		t.Errorf("ratio = %f, want ~0.1667", ratio)
	}

	// Exactly on a level boundary -> ratio 0 at the new level.
	level, into, _, ratio = ProgressForXP(400)
	if level != 3 || into != 0 || ratio != 0 {
		t.Errorf("at boundary 400: level=%d into=%d ratio=%f, want 3/0/0", level, into, ratio)
	}
}
