package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Comment is a review note the user left on a line or a range of a file.
type Comment struct {
	ID        string `json:"id"`
	File      string `json:"file"` // relative to the repo root
	Line      int    `json:"line"`
	EndLine   int    `json:"end_line"`
	Text      string `json:"text"`
	Snippet   string `json:"snippet,omitempty"`
	CreatedAt string `json:"created_at"`
	SentAt    string `json:"sent_at,omitempty"`
	// TargetBlock is the Rex pane of the agent the note is for: the agent
	// in the editor's tab. Empty when the note was made outside Rex.
	TargetBlock string `json:"target_block,omitempty"`
	// Drift is set when the note is sent: "moved" when the reviewed code is
	// now at other lines, "changed" when it's no longer in the file, "deleted"
	// when the file is gone. For moved and changed notes ReviewedLine keeps
	// the old start and Line/EndLine point at where the code is now (0 when a
	// changed note's code can't be found); Current holds that changed code.
	Drift        string `json:"drift,omitempty"`
	ReviewedLine int    `json:"reviewed_line,omitempty"`
	Current      string `json:"current,omitempty"`
}

// outsideRex stands for an agent not running in a Rex pane: it matches only
// notes made outside Rex (no TargetBlock).
const outsideRex = "outside-rex"

// For reports whether the note belongs to the agent in Rex pane block. An
// empty block means any agent; outsideRex only matches notes with no pane.
func (c Comment) For(block string) bool {
	return c.TargetBlock == "" || block == "" || c.TargetBlock == block
}

func (c Comment) Pending() bool { return c.SentAt == "" }

func (c Comment) Range() string {
	if c.EndLine > c.Line {
		return fmt.Sprintf("%d-%d", c.Line, c.EndLine)
	}
	return fmt.Sprintf("%d", c.Line)
}

type commentStore struct {
	repo string
	path string
	lock *os.File
}

func openComments(repo string) (*commentStore, error) {
	sd, err := stateDir(repo)
	if err != nil {
		return nil, err
	}
	lf, err := os.OpenFile(filepath.Join(sd, "comments.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		lf.Close()
		return nil, err
	}
	return &commentStore{repo: repo, path: filepath.Join(sd, "comments.json"), lock: lf}, nil
}

func (s *commentStore) Close() {
	if s.lock != nil {
		_ = syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
		s.lock.Close()
	}
}

func (s *commentStore) Load() []Comment {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var cs []Comment
	_ = json.Unmarshal(b, &cs)
	return cs
}

func (s *commentStore) Save(cs []Comment) error {
	if cs == nil {
		cs = []Comment{}
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].File != cs[j].File {
			return cs[i].File < cs[j].File
		}
		return cs[i].Line < cs[j].Line
	})
	return writeJSONAtomic(s.path, cs)
}

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// relToRepo turns a path (absolute or relative to cwd) into a repo-relative one.
func relToRepo(repo, file string) string {
	abs := file
	if !filepath.IsAbs(abs) {
		wd, _ := os.Getwd()
		abs = filepath.Join(wd, file)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	rr := repo
	if real, err := filepath.EvalSymlinks(repo); err == nil {
		rr = real
	}
	if rel, err := filepath.Rel(rr, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}

func readSnippet(repo, rel string, from, to int) string {
	b, err := os.ReadFile(filepath.Join(repo, rel))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	if from < 1 {
		from = 1
	}
	if to < from {
		to = from
	}
	if to-from > 60 {
		to = from + 60
	}
	if from > len(lines) {
		return ""
	}
	if to > len(lines) {
		to = len(lines)
	}
	return strings.Join(lines[from-1:to], "\n")
}

func addComment(repo, file string, line, end int, text, snippet, target string) (*Comment, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty comment")
	}
	if end < line {
		end = line
	}
	rel := relToRepo(repo, file)
	if snippet == "" {
		snippet = readSnippet(repo, rel, line, end)
	}
	c := Comment{
		ID: newID(), File: rel, Line: line, EndLine: end, Text: text,
		Snippet:     snippet,
		CreatedAt:   time.Now().Format(time.RFC3339),
		TargetBlock: target,
	}
	s, err := openComments(repo)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	cs := append(s.Load(), c)
	return &c, s.Save(cs)
}

func langFor(file string) string {
	ext := strings.TrimPrefix(filepath.Ext(file), ".")
	switch ext {
	case "ts", "tsx", "js", "jsx", "go", "rs", "py", "rb", "lua", "swift", "kt", "java", "c", "h", "cpp", "md", "json", "yaml", "yml", "toml", "sh", "css", "html", "sql":
		return ext
	}
	return ""
}

// formatComments renders comments as a prompt an agent can act on.
func formatComments(cs []Comment) string {
	var b strings.Builder
	b.WriteString("Review comments I left on the code (file:line, the code, then my comment). Address each one:\n")
	for i, c := range cs {
		lang := langFor(c.File)
		switch c.Drift {
		case "moved":
			fmt.Fprintf(&b, "\n%d. %s:%s (was line %d when I reviewed it; the code is unchanged)\n", i+1, c.File, c.Range(), c.ReviewedLine)
		case "changed":
			if c.Line > 0 {
				fmt.Fprintf(&b, "\n%d. %s, reviewed at line %d. This code changed after I reviewed it; it now looks to be at line %s. Check the comment still applies before acting on it.\n", i+1, c.File, c.ReviewedLine, c.Range())
			} else {
				fmt.Fprintf(&b, "\n%d. %s, reviewed at line %d. This code changed after I reviewed it and I can't find it in the file now. Check the comment still applies before acting on it.\n", i+1, c.File, c.ReviewedLine)
			}
		case "deleted":
			fmt.Fprintf(&b, "\n%d. %s:%s. This file was deleted or renamed after I reviewed it: check the comment still applies before acting on it.\n", i+1, c.File, c.Range())
		default:
			fmt.Fprintf(&b, "\n%d. %s:%s\n", i+1, c.File, c.Range())
		}
		if c.Snippet != "" {
			if c.Drift == "changed" || c.Drift == "deleted" {
				b.WriteString("The code I reviewed:\n")
			}
			writeCode(&b, lang, c.Snippet)
		}
		if c.Drift == "changed" && c.Current != "" {
			b.WriteString("What's there now:\n")
			writeCode(&b, lang, c.Current)
		}
		fmt.Fprintf(&b, "Comment: %s\n", c.Text)
	}
	return stripControl(b.String())
}

// writeCode fences code with more backticks than any run inside it, so code
// that itself contains ``` can't close the block early.
func writeCode(b *strings.Builder, lang, code string) {
	fence, run := 3, 0
	for _, r := range code {
		if r == '`' {
			run++
			if run >= fence {
				fence = run + 1
			}
		} else {
			run = 0
		}
	}
	f := strings.Repeat("`", fence)
	fmt.Fprintf(b, "%s%s\n%s\n%s\n", f, lang, code, f)
}

// stripControl drops control characters but newline and tab. The text is
// pasted into the agent's terminal, where an ESC from file content (say
// ESC[201~, the end of a bracketed paste) would be read as keystrokes.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

// trimLines splits text into lines without trailing whitespace, so a
// reformatted line end doesn't count as a change.
func trimLines(s string) []string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range ls {
		ls[i] = strings.TrimRight(ls[i], " \t\r")
	}
	return ls
}

// checkDrift compares the code a note was made on with the file as it is
// now and records what happened to it (see Comment.Drift). If the reviewed
// lines appear elsewhere, the note follows the copy nearest to where it was.
func checkDrift(repo string, c *Comment) {
	c.Drift, c.ReviewedLine, c.Current = "", 0, ""
	if strings.TrimSpace(c.Snippet) == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(repo, c.File))
	if err != nil {
		if os.IsNotExist(err) {
			c.Drift = "deleted"
		}
		return
	}
	file := trimLines(string(data))
	want := trimLines(c.Snippet)
	matchAt := func(i int) bool {
		if i < 0 || i+len(want) > len(file) {
			return false
		}
		for j, l := range want {
			if file[i+j] != l {
				return false
			}
		}
		return true
	}
	start := c.Line - 1
	if matchAt(start) {
		return
	}
	best := -1
	for i := 0; i+len(want) <= len(file); i++ {
		if matchAt(i) && (best < 0 || abs(i-start) < abs(best-start)) {
			best = i
		}
	}
	if best >= 0 {
		span := c.EndLine - c.Line
		c.Drift, c.ReviewedLine = "moved", c.Line
		c.Line = best + 1
		c.EndLine = c.Line + span
		return
	}
	// Not there any more. Find where it most likely went: the nearest copy of
	// its first line (e.g. the func signature), or else of its last line, and
	// show that much code from there. Lines at the old numbers may well be
	// unrelated code by now, so they're never shown as "the same place".
	c.Drift, c.ReviewedLine = "changed", c.Line
	span := c.EndLine - c.Line
	for k, l := range []string{want[0], want[len(want)-1]} {
		if strings.TrimSpace(l) == "" {
			continue
		}
		at := -1
		for i := range file {
			if file[i] == l && (at < 0 || abs(i-start) < abs(at-start)) {
				at = i
			}
		}
		if at < 0 {
			continue
		}
		if k == 1 { // matched the last line: the block ends there
			at -= len(want) - 1
			if at < 0 {
				at = 0
			}
		}
		c.Line = at + 1
		c.EndLine = c.Line + span
		c.Current = readSnippet(repo, c.File, c.Line, c.EndLine)
		return
	}
	c.Line, c.EndLine = 0, 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// takePending returns the pending comments for the agent in Rex pane block
// (all of them when block is empty) and marks them sent.
func takePending(repo, block string) ([]Comment, error) {
	s, err := openComments(repo)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	cs := s.Load()
	var pending []Comment
	now := time.Now().Format(time.RFC3339)
	for i := range cs {
		if cs[i].Pending() && cs[i].For(block) {
			checkDrift(repo, &cs[i])
			pending = append(pending, cs[i])
			cs[i].SentAt = now
		}
	}
	if len(pending) == 0 {
		return nil, nil
	}
	return pending, s.Save(cs)
}

// editorTarget finds the agent pane for a note made in the editor: the
// agent in the editor pane's tab. It reads the tab from the environment the
// editor runs in (REXMOD_* when rexmod opened it, REX_* in any Rex pane).
func editorTarget() string {
	if s, w := os.Getenv("REXMOD_SESSION"), os.Getenv("REXMOD_WINDOW"); s != "" {
		if t, err := resolveTarget(s, w); err == nil {
			return t.AgentBlockID
		}
	}
	s, b := os.Getenv("REX_SESSION"), os.Getenv("REX_BLOCK")
	if s == "" || b == "" {
		return ""
	}
	v, err := sessionView(s)
	if err != nil {
		return ""
	}
	for _, w := range v.Windows {
		for _, blk := range w.blocks() {
			if blk.BlockID == b || strings.HasSuffix(blk.BlockID, b) {
				if t, err := resolveTarget(s, w.WindowID); err == nil {
					return t.AgentBlockID
				}
			}
		}
	}
	return ""
}
