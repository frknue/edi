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

func TestGoldForXP(t *testing.T) {
	cases := []struct {
		xp   int64
		want int64
	}{
		{-5, 0}, {0, 0}, {1, 1}, {5, 1}, {9, 1}, {10, 1}, {11, 1}, {19, 1},
		{20, 2}, {40, 4}, {100, 10}, {2520, 252},
	}
	for _, c := range cases {
		if got := GoldForXP(c.xp); got != c.want {
			t.Errorf("GoldForXP(%d) = %d, want %d", c.xp, got, c.want)
		}
	}
}

func TestDailyDecay(t *testing.T) {
	cases := []struct {
		total int64
		want  int64
	}{
		{-10, 0}, {0, 0}, {1, 1}, {3, 3}, {4, 4}, {5, 5}, {6, 5}, {100, 5},
		{499, 5}, {500, 5}, {600, 6}, {2520, 25}, {10000, 100},
	}
	for _, c := range cases {
		if got := DailyDecay(c.total); got != c.want {
			t.Errorf("DailyDecay(%d) = %d, want %d", c.total, got, c.want)
		}
	}
}

func TestDecayFloor(t *testing.T) {
	cases := []struct {
		peak int64
		want int64
	}{
		{0, 0},      // peak level 1 -> floor level -1 -> clamp 0
		{99, 0},     // level 1
		{100, 0},    // level 2 -> floor level 0 -> clamp 0
		{400, 0},    // level 3 -> floor level 1 -> 0 XP
		{900, 100},  // level 4 -> floor level 2 -> 100 XP
		{1600, 400}, // level 5 -> floor level 3 -> 400 XP
	}
	for _, c := range cases {
		if got := DecayFloor(c.peak); got != c.want {
			t.Errorf("DecayFloor(%d) = %d, want %d", c.peak, got, c.want)
		}
	}
}
