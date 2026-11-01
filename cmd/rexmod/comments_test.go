package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func driftRepo(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckDrift(t *testing.T) {
	reviewed := "one\ntwo\nthree\nfour\n"
	note := func() Comment {
		return Comment{File: "a.go", Line: 2, EndLine: 3, Snippet: "two\nthree", Text: "x"}
	}

	t.Run("unchanged", func(t *testing.T) {
		c := note()
		checkDrift(driftRepo(t, reviewed), &c)
		if c.Drift != "" || c.Line != 2 {
			t.Fatalf("got %q at %d", c.Drift, c.Line)
		}
	})

	t.Run("trailing whitespace only", func(t *testing.T) {
		c := note()
		checkDrift(driftRepo(t, "one\ntwo  \nthree\t\nfour\n"), &c)
		if c.Drift != "" {
			t.Fatalf("got %q", c.Drift)
		}
	})

	t.Run("moved", func(t *testing.T) {
		c := note()
		checkDrift(driftRepo(t, "new\nnew\none\ntwo\nthree\nfour\n"), &c)
		if c.Drift != "moved" || c.Line != 4 || c.EndLine != 5 || c.ReviewedLine != 2 {
			t.Fatalf("got %q %d-%d was %d", c.Drift, c.Line, c.EndLine, c.ReviewedLine)
		}
	})

	t.Run("moved picks the nearest copy", func(t *testing.T) {
		c := Comment{File: "a.go", Line: 10, EndLine: 10, Snippet: "x", Text: "x"}
		checkDrift(driftRepo(t, "x\n"+strings.Repeat("-\n", 9)+"y\nx\n"), &c)
		if c.Drift != "moved" || c.Line != 12 {
			t.Fatalf("got %q at %d", c.Drift, c.Line)
		}
	})

	t.Run("changed", func(t *testing.T) {
		c := note()
		checkDrift(driftRepo(t, "one\nTWO\nthree\nfour\n"), &c)
		if c.Drift != "changed" || c.Current != "TWO\nthree" || c.Line != 2 {
			t.Fatalf("got %q current %q", c.Drift, c.Current)
		}
	})

	t.Run("deleted", func(t *testing.T) {
		c := note()
		checkDrift(t.TempDir(), &c)
		if c.Drift != "deleted" {
			t.Fatalf("got %q", c.Drift)
		}
	})

	t.Run("no snippet", func(t *testing.T) {
		c := note()
		c.Snippet = ""
		checkDrift(driftRepo(t, "zzz\n"), &c)
		if c.Drift != "" {
			t.Fatalf("got %q", c.Drift)
		}
	})
}

func TestFormatCommentsDrift(t *testing.T) {
	out := formatComments([]Comment{
		{File: "a.go", Line: 4, EndLine: 4, Snippet: "two", Text: "m", Drift: "moved", ReviewedLine: 2},
		{File: "b.go", Line: 2, EndLine: 2, Snippet: "old", Current: "new", Text: "c", Drift: "changed"},
	})
	for _, want := range []string{
		"a.go:4 (was line 2 when I reviewed it; the code is unchanged)",
		"b.go:2. This code changed after I reviewed it",
		"The code I reviewed:\n```go\nold\n```",
		"Those lines now:\n```go\nnew\n```",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
