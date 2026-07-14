// Command edi-cli is a terminal client for the Life RPG API. It is a thin
// HTTP client over the SAME REST endpoints the web UI uses — no direct DB access —
// demonstrating the "one API for every client" architecture.
//
// Usage:
//
//	edi-cli [--addr URL] <command> [args]
//
// Commands:
//
//	dashboard                       Character, attributes, today's quests, streak
//	quests [--type t] [--status s]  List quests
//	add --title T [flags]           Create a quest (--type --difficulty --desc --reward k=v)
//	complete <id>                   Complete a quest (shows XP + level-ups)
//	skip <id> | archive <id>        Skip / archive a quest
//	journal                         List recent reflections
//	journal-add --mood N --energy N [--notes "..."]
//	suggest                         List pending agent suggestions
//	suggest-gen                     Generate rule-based suggestions
//	suggest-accept <id> | suggest-dismiss <id>
//	shop                            List reward shop items
//	shop-add --name N --price P     Add a reward to the shop
//	buy <id>                        Purchase a shop item (spends gold)
//	gold                            Gold balance + recent ledger
//	ward <attribute>                Buy a 7-day decay ward for an attribute (30g)
//	rest on|off                     Pause/resume all attribute decay
//	tools                           List the agent tool catalog
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"edi/internal/apiclient"
	"edi/internal/models"
)

func main() {
	addr := envOr("EDI_API", "http://localhost:8080")
	// A leading global --addr before the subcommand.
	args := os.Args[1:]
	for len(args) >= 2 && (args[0] == "--addr" || args[0] == "-addr") {
		addr = args[1]
		args = args[2:]
	}
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	c := apiclient.New(addr)
	c.Token = os.Getenv("EDI_TOKEN") // optional bearer auth (server started with EDI_TOKEN)
	cmd, rest := args[0], args[1:]
	if err := run(c, cmd, rest); err != nil {
		fmt.Fprintln(os.Stderr, red("error: ")+err.Error())
		os.Exit(1)
	}
}

func run(c *apiclient.Client, cmd string, args []string) error {
	switch cmd {
	case "dashboard", "dash":
		return cmdDashboard(c)
	case "quests":
		return cmdQuests(c, args)
	case "add":
		return cmdAdd(c, args)
	case "complete":
		return cmdComplete(c, args)
	case "subtask":
		return cmdToggleSubtask(c, args)
	case "skip":
		return cmdSimpleQuest(c, args, c.SkipQuest, "skipped")
	case "archive":
		return cmdSimpleQuest(c, args, c.ArchiveQuest, "archived")
	case "journal":
		return cmdJournal(c)
	case "journal-add":
		return cmdJournalAdd(c, args)
	case "journal-rm":
		return cmdJournalRm(c, args)
	case "suggest":
		return cmdSuggest(c)
	case "suggest-gen":
		return cmdSuggestGen(c)
	case "suggest-accept":
		return cmdSuggestAccept(c, args)
	case "suggest-dismiss":
		return cmdSuggestDismiss(c, args)
	case "shop":
		return cmdShop(c)
	case "shop-add":
		return cmdShopAdd(c, args)
	case "buy":
		return cmdBuy(c, args)
	case "gold":
		return cmdGold(c)
	case "ward":
		return cmdWard(c, args)
	case "rest":
		return cmdRest(c, args)
	case "tools":
		return cmdTools(c)
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// --- commands ---------------------------------------------------------------

func cmdDashboard(c *apiclient.Client) error {
	d, err := c.Dashboard()
	if err != nil {
		return err
	}
	ch := d.Character
	fmt.Printf("\n  %s  %s  Lv %s  (%d/%d XP to next)\n",
		bold(ch.Name), dim("·"), bold(strconv.Itoa(ch.Level)), ch.XPIntoLevel, ch.XPForNextLevel)
	fmt.Printf("  %s   streak %s  ·  today %d/%d\n\n",
		bar(ch.Progress, 24), bold(strconv.Itoa(d.Streak.Current)), d.DailyProgress.CompletedToday, d.DailyProgress.Goal)

	fmt.Println(dim("  ATTRIBUTES"))
	for _, a := range d.Attributes {
		fmt.Printf("  %-14s Lv%-2d %s %s", a.Name, a.Level, bar(a.Progress, 16), dim(fmt.Sprintf("%d/%d", a.XPIntoLevel, a.XPForNextLevel)))
		if a.Decay != nil && a.Decay.State != "fresh" {
			fmt.Printf("  [%s", a.Decay.State)
			if a.Decay.State == "decaying" {
				fmt.Printf(": %dd idle, -%d/day", a.Decay.IdleDays, a.Decay.ProjectedDailyLoss)
			}
			fmt.Print("]")
		}
		fmt.Println()
	}

	fmt.Println("\n" + dim("  TODAY'S QUESTS"))
	if len(d.TodayQuests) == 0 {
		fmt.Println("  (none active)")
	}
	for _, q := range d.TodayQuests {
		fmt.Printf("  %s %s  %s %s\n", dim(fmt.Sprintf("#%d", q.ID)), q.Title, tag(q.Type), dim(rewardStr(q.AttributeRewards)))
	}
	if d.RecommendedQuest != nil {
		fmt.Printf("\n  %s #%d %s\n", green("→ recommended:"), d.RecommendedQuest.ID, d.RecommendedQuest.Title)
	}
	fmt.Println()
	return nil
}

func cmdQuests(c *apiclient.Client, args []string) error {
	fs := flag.NewFlagSet("quests", flag.ContinueOnError)
	typ := fs.String("type", "", "filter by type")
	status := fs.String("status", "", "filter by status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	qs, err := c.ListQuests(*typ, *status)
	if err != nil {
		return err
	}
	if len(qs) == 0 {
		fmt.Println("(no quests match)")
		return nil
	}
	for _, q := range qs {
		fmt.Printf("  %s %-34s %-9s %-9s %s\n", dim(fmt.Sprintf("#%d", q.ID)), q.Title, tag(q.Type), dim(q.Status), dim(rewardStr(q.AttributeRewards)))
		for _, st := range q.Subtasks {
			box := "☐"
			if st.Done {
				box = green("☑")
			}
			fmt.Printf("      %s %s %s %s\n", box, dim(fmt.Sprintf("#%d", st.ID)), st.Title, dim(rewardStr(st.AttributeRewards)))
		}
	}
	return nil
}

func cmdToggleSubtask(c *apiclient.Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: subtask <quest_id> <subtask_id>")
	}
	questID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("bad quest id %q", args[0])
	}
	subtaskID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("bad subtask id %q", args[1])
	}
	st, err := c.ToggleSubtask(questID, subtaskID)
	if err != nil {
		return err
	}
	state := "unchecked"
	if st.Done {
		state = "checked"
	}
	fmt.Printf("%s %s subtask #%d %q %s\n", green("✓"), state, st.ID, st.Title, dim(rewardStr(st.AttributeRewards)))
	return nil
}

func cmdAdd(c *apiclient.Client, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	title := fs.String("title", "", "quest title (required)")
	desc := fs.String("desc", "", "description")
	typ := fs.String("type", "daily", "type: daily|weekly|main|side|boss|recovery")
	diff := fs.String("difficulty", "easy", "difficulty: trivial|easy|medium|hard|boss")
	var rewards rewardFlag
	fs.Var(&rewards, "reward", "attribute reward k=v (repeatable), e.g. --reward strength=40")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *title == "" {
		return fmt.Errorf("--title is required")
	}
	in := models.QuestInput{
		Title: *title, Description: *desc, Type: *typ, Difficulty: *diff,
		AttributeRewards: rewards.m,
	}
	q, err := c.CreateQuest(in)
	if err != nil {
		return err
	}
	fmt.Printf("%s created quest #%d %q %s\n", green("✓"), q.ID, q.Title, dim(rewardStr(q.AttributeRewards)))
	return nil
}

func cmdComplete(c *apiclient.Client, args []string) error {
	id, err := argID(args)
	if err != nil {
		return err
	}
	res, err := c.CompleteQuest(id)
	if err != nil {
		return err
	}
	var total int64
	parts := []string{}
	for _, e := range res.XPEvents {
		total += e.Amount
		parts = append(parts, fmt.Sprintf("+%d %s", e.Amount, e.AttributeName))
	}
	fmt.Printf("%s completed %q  %s\n", green("✓"), res.Quest.Title, bold(fmt.Sprintf("+%d XP", total)))
	if len(parts) > 0 {
		fmt.Println("  " + dim(strings.Join(parts, "  ")))
	}
	for _, lu := range res.LevelUps {
		fmt.Printf("  %s %s reached Lv %d\n", green("⤴"), lu.AttributeName, lu.ToLevel)
	}
	return nil
}

func cmdSimpleQuest(c *apiclient.Client, args []string, fn func(int64) (models.Quest, error), verb string) error {
	id, err := argID(args)
	if err != nil {
		return err
	}
	q, err := fn(id)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s quest #%d %q\n", green("✓"), verb, q.ID, q.Title)
	return nil
}

func cmdJournal(c *apiclient.Client) error {
	entries, err := c.ListJournal(20)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("(no reflections yet)")
		return nil
	}
	for _, e := range entries {
		fmt.Printf("  %s mood %d energy %d  %s\n", dim(e.CreatedAt.Format("Jan 2 15:04")), e.Mood, e.Energy, e.Notes)
	}
	return nil
}

func cmdJournalAdd(c *apiclient.Client, args []string) error {
	fs := flag.NewFlagSet("journal-add", flag.ContinueOnError)
	mood := fs.Int("mood", 0, "mood 1-10 (required)")
	energy := fs.Int("energy", 0, "energy 1-10 (required)")
	notes := fs.String("notes", "", "free-text notes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := c.CreateJournal(models.JournalInput{Mood: *mood, Energy: *energy, Notes: *notes})
	if err != nil {
		return err
	}
	fmt.Printf("%s saved reflection #%d (mood %d, energy %d)\n", green("✓"), res.Entry.ID, res.Entry.Mood, res.Entry.Energy)
	if len(res.XPEvents) > 0 {
		var total int64
		for _, ev := range res.XPEvents {
			total += ev.Amount
		}
		fmt.Printf("  %s first reflection today: %s\n", green("★"), bold(fmt.Sprintf("+%d XP", total)))
	}
	return nil
}

func cmdJournalRm(c *apiclient.Client, args []string) error {
	id, err := argID(args)
	if err != nil {
		return err
	}
	if err := c.DeleteJournal(id); err != nil {
		return err
	}
	fmt.Printf("%s deleted reflection #%d\n", green("✓"), id)
	return nil
}

func cmdSuggest(c *apiclient.Client) error {
	ss, err := c.ListSuggestions("pending")
	if err != nil {
		return err
	}
	if len(ss) == 0 {
		fmt.Println("(no pending suggestions — try `suggest-gen`)")
		return nil
	}
	for _, s := range ss {
		fmt.Printf("  %s %s\n     %s\n     %s %s %s\n", dim(fmt.Sprintf("#%d", s.ID)), bold(s.Title), dim(s.Reason),
			dim("→"), s.SuggestedQuest.Title, dim(rewardStr(s.SuggestedQuest.AttributeRewards)))
	}
	return nil
}

func cmdSuggestGen(c *apiclient.Client) error {
	ss, err := c.GenerateSuggestions()
	if err != nil {
		return err
	}
	fmt.Printf("%s %d pending suggestion(s)\n", green("✓"), len(ss))
	return cmdSuggest(c)
}

func cmdSuggestAccept(c *apiclient.Client, args []string) error {
	id, err := argID(args)
	if err != nil {
		return err
	}
	q, err := c.AcceptSuggestion(id)
	if err != nil {
		return err
	}
	fmt.Printf("%s accepted → created quest #%d %q\n", green("✓"), q.ID, q.Title)
	return nil
}

func cmdSuggestDismiss(c *apiclient.Client, args []string) error {
	id, err := argID(args)
	if err != nil {
		return err
	}
	if _, err := c.DismissSuggestion(id); err != nil {
		return err
	}
	fmt.Printf("%s dismissed suggestion #%d\n", green("✓"), id)
	return nil
}

func cmdTools(c *apiclient.Client) error {
	tools, err := c.ListTools()
	if err != nil {
		return err
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	fmt.Printf("%s agent tools (same path the MCP bridge / AI agent uses)\n", bold(strconv.Itoa(len(tools))))
	for _, t := range tools {
		fmt.Printf("  %-22s %s\n", green(t.Name), dim(t.Description))
	}
	return nil
}

func cmdShop(c *apiclient.Client) error {
	items, err := c.ListShopItems()
	if err != nil {
		return err
	}
	dash, err := c.Dashboard()
	if err != nil {
		return err
	}
	fmt.Printf("Gold: %dg\n\n", dash.GoldBalance)
	if len(items) == 0 {
		fmt.Println("The shop is empty. Add rewards with: edi-cli shop-add --name \"Gaming evening\" --price 50")
		return nil
	}
	for _, it := range items {
		fmt.Printf("  [%d] %-40s %6dg\n", it.ID, it.Name, it.Price)
	}
	return nil
}

func cmdShopAdd(c *apiclient.Client, args []string) error {
	fs := flag.NewFlagSet("shop-add", flag.ExitOnError)
	name := fs.String("name", "", "reward name (required)")
	price := fs.Int64("price", 0, "gold price (required, > 0)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	it, err := c.CreateShopItem(models.ShopItemInput{Name: *name, Price: *price})
	if err != nil {
		return err
	}
	fmt.Printf("Added [%d] %s — %dg\n", it.ID, it.Name, it.Price)
	return nil
}

func cmdBuy(c *apiclient.Client, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: buy <item-id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id %q", args[0])
	}
	res, err := c.PurchaseShopItem(id)
	if err != nil {
		return err
	}
	fmt.Printf("Purchased %q for %dg. Balance: %dg. Enjoy it — you earned it.\n", res.Item.Name, res.Item.Price, res.Balance)
	return nil
}

func cmdGold(c *apiclient.Client) error {
	dash, err := c.Dashboard()
	if err != nil {
		return err
	}
	fmt.Printf("Gold: %dg\n\nRecent ledger:\n", dash.GoldBalance)
	events, err := c.ListGoldEvents(15, "")
	if err != nil {
		return err
	}
	for _, e := range events {
		sign := "+"
		if e.Amount < 0 {
			sign = ""
		}
		fmt.Printf("  %s%dg  %-9s %s\n", sign, e.Amount, e.Source, e.Label)
	}
	return nil
}

func cmdWard(c *apiclient.Client, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ward <attribute-key>")
	}
	res, err := c.WardAttribute(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Warded %s until %s. Balance: %dg\n",
		res.Ward.AttributeKey, res.Ward.ExpiresAt.Local().Format("2006-01-02 15:04"), res.Balance)
	return nil
}

func cmdRest(c *apiclient.Client, args []string) error {
	if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
		return fmt.Errorf("usage: rest on|off")
	}
	state, err := c.SetRestMode(args[0] == "on")
	if err != nil {
		return err
	}
	if state.On {
		fmt.Println("Rest mode ON — decay paused. Recover well.")
	} else {
		fmt.Println("Rest mode OFF — idle clocks restarted from now.")
	}
	return nil
}

// --- helpers ----------------------------------------------------------------

type rewardFlag struct{ m map[string]int64 }

func (r *rewardFlag) String() string { return rewardStr(r.m) }
func (r *rewardFlag) Set(v string) error {
	if r.m == nil {
		r.m = map[string]int64{}
	}
	k, val, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("reward must be key=value, got %q", v)
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return fmt.Errorf("reward value for %q must be an integer", k)
	}
	r.m[strings.TrimSpace(k)] = n
	return nil
}

func rewardStr(m map[string]int64) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("+%d %s", m[k], k))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func argID(args []string) (int64, error) {
	if len(args) < 1 {
		return 0, fmt.Errorf("expected a quest/suggestion id")
	}
	return strconv.ParseInt(args[0], 10, 64)
}

func bar(ratio float64, width int) string {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio*float64(width) + 0.5)
	return "[" + strings.Repeat("█", filled) + strings.Repeat("·", width-filled) + "]"
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func usage() {
	fmt.Fprint(os.Stderr, `edi-cli — terminal client for the Life RPG API

  usage: edi-cli [--addr URL] <command> [args]
  (default addr: $EDI_API or http://localhost:8080)

commands:
  dashboard                          character, attributes, today's quests, streak
  quests [--type t] [--status s]     list quests
  add --title T [--type --difficulty --desc --reward k=v ...]
  complete <id> | skip <id> | archive <id>
  subtask <quest_id> <subtask_id>    toggle a bonus objective
  journal                            list recent reflections
  journal-add --mood N --energy N [--notes "..."]
  suggest | suggest-gen | suggest-accept <id> | suggest-dismiss <id>
  shop                            List reward shop items
  shop-add --name N --price P     Add a reward to the shop
  buy <id>                        Purchase a shop item (spends gold)
  gold                            Gold balance + recent ledger
  ward <attribute>                   buy a 7-day decay ward for an attribute (30g)
  rest on|off                        pause/resume all attribute decay
  tools                              list the agent tool catalog
`)
}

// --- minimal ANSI (skipped when not a TTY) ----------------------------------

var useColor = isTTY()

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}

func color(code, s string) string {
	if !useColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bold(s string) string  { return color("1", s) }
func dim(s string) string   { return color("2", s) }
func green(s string) string { return color("32", s) }
func red(s string) string   { return color("31", s) }

func tag(s string) string {
	if s == "boss" {
		return color("31", "["+s+"]")
	}
	return dim("[" + s + "]")
}
