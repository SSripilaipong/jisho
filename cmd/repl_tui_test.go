package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/creack/pty"
	"github.com/shsnail/jisho/internal/model"
	"github.com/shsnail/jisho/internal/query"
)

type fakeQuerier struct{}

func (fakeQuerier) SearchWords(_ context.Context, _ string, _ query.SearchOpts) ([]model.Word, error) {
	return testWords("alpha"), nil
}
func (fakeQuerier) SearchNames(context.Context, string) ([]model.Name, error) { return nil, nil }
func (fakeQuerier) LookupKanji(_ context.Context, s string) (*model.Kanji, error) {
	return &model.Kanji{Literal: s, MeaningsEN: []string{"food", "to eat"}}, nil
}
func (fakeQuerier) FilterKanjiByRadicals(context.Context, []string) ([]model.Kanji, error) {
	return []model.Kanji{{Literal: "食", MeaningsEN: []string{"food"}}}, nil
}

func press(s string) tea.KeyPressMsg {
	special := map[string]rune{"tab": tea.KeyTab, "enter": tea.KeyEnter, "esc": tea.KeyEscape, "left": tea.KeyLeft, "right": tea.KeyRight, "up": tea.KeyUp, "down": tea.KeyDown}
	if r, ok := special[s]; ok {
		return tea.KeyPressMsg(tea.Key{Code: r})
	}
	if strings.HasPrefix(s, "ctrl+") {
		r := []rune(strings.TrimPrefix(s, "ctrl+"))[0]
		return tea.KeyPressMsg(tea.Key{Code: r, Mod: tea.ModCtrl})
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg(tea.Key{Code: r, Text: s})
}

func updateModel(t *testing.T, m replModel, msg tea.Msg) replModel {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(replModel)
}

func selectionModel() replModel {
	m := newREPLModel(context.Background(), fakeQuerier{}, "")
	m.doc = newDocument()
	m.doc.selectable("  1. ", "take care of café")
	m.field = 0
	m.pos = 0
	m.layout()
	return m
}

func TestTUIFocusFindAndVisualMotions(t *testing.T) {
	m := selectionModel()
	m = updateModel(t, m, press("tab"))
	if m.focus != focusResults {
		t.Fatal("Tab did not focus results")
	}
	m = updateModel(t, m, press("/"))
	if m.focus != focusFind {
		t.Fatal("/ did not start find")
	}
	for _, r := range "care" {
		m = updateModel(t, m, press(string(r)))
	}
	m = updateModel(t, m, press("enter"))
	if m.field != 0 || m.pos != 5 {
		t.Fatalf("find cursor=%d:%d", m.field, m.pos)
	}
	m = updateModel(t, m, press("v"))
	m = updateModel(t, m, press("e"))
	if got := m.lookupText(); got != "care" {
		t.Fatalf("selection=%q", got)
	}
	// Reverse through the anchor, then verify arrows exactly mirror h/l.
	for range 6 {
		m = updateModel(t, m, press("left"))
	}
	if got := m.lookupText(); got != "e c" {
		t.Fatalf("reverse selection=%q", got)
	}
	before := m.pos
	m = updateModel(t, m, press("right"))
	if m.pos != before+1 {
		t.Fatal("right did not mirror l")
	}
	m = updateModel(t, m, press("esc"))
	if m.visual == true || m.focus != focusResults {
		t.Fatal("first Esc should only leave visual")
	}
	m = updateModel(t, m, press("esc"))
	if m.focus != focusPrompt {
		t.Fatal("second Esc should focus prompt")
	}
}

func TestTUIFieldBoundariesAndUnicode(t *testing.T) {
	m := newREPLModel(context.Background(), fakeQuerier{}, "")
	m.doc = newDocument()
	m.doc.selectable("", "e\u0301lan")
	m.doc.selectable("", "second meaning")
	m.field = 0
	m.pos = 0
	m.focus = focusResults
	if len(graphemes(m.doc.fields[0])) != 4 {
		t.Fatalf("combining mark was split: %#v", graphemes(m.doc.fields[0]))
	}
	m = updateModel(t, m, press("v"))
	m = updateModel(t, m, press("k"))
	if m.field != 0 {
		t.Fatal("visual selection crossed field")
	}
	for range 8 {
		m = updateModel(t, m, press("right"))
	}
	if m.pos != 3 {
		t.Fatalf("cursor escaped field: %d", m.pos)
	}
	m.visual = false
	m = updateModel(t, m, press("down"))
	if m.field != 1 {
		t.Fatal("Down did not move to next field")
	}
}

func TestTUIStaleResultsAndThaiStates(t *testing.T) {
	m := selectionModel()
	m.requestID = 2
	m.loading = true
	m = updateModel(t, m, searchResultMsg{id: 1, doc: resultDocument{fields: []string{"stale"}}})
	if !m.loading || m.doc.fields[0] != "take care of café" {
		t.Fatal("stale result applied")
	}
	m = updateModel(t, m, searchResultMsg{id: 2, doc: resultDocument{fields: []string{"fresh"}, lines: []resultLine{{text: "fresh", field: 0}}}})
	if m.loading || m.doc.fields[0] != "fresh" {
		t.Fatal("current result ignored")
	}
	m.requestID = 3
	m.thai = thaiPane{open: true, loading: true, text: "unknown phrase"}
	m = updateModel(t, m, thaiResultMsg{id: 3, text: "unknown phrase"})
	if !m.thai.phraseMiss {
		t.Fatal("phrase miss not offered")
	}
	m.thai.entries = []model.ThaiEntry{{English: "food", Thai: "อาหาร", PartOfSpeech: "N", Related: "ของกิน"}}
	m.thai.phraseMiss = false
	if strings.Contains(m.renderThai(8), "related:") {
		t.Fatal("details visible initially")
	}
	m = updateModel(t, m, press("d"))
	if !strings.Contains(m.renderThai(8), "related:") {
		t.Fatal("details did not toggle")
	}
	m = updateModel(t, m, press("esc"))
	if m.thai.open {
		t.Fatal("Thai pane did not close")
	}
}

func TestTUINewResultsResetViewportToTop(t *testing.T) {
	m := newREPLModel(context.Background(), fakeQuerier{}, "")
	m.width, m.height = 40, 8
	m.doc = newDocument()
	for range 20 {
		m.doc.plain("old result")
	}
	m.refreshViewport()
	m.viewport.SetYOffset(8)
	if m.viewport.YOffset() == 0 {
		t.Fatal("test setup did not scroll viewport")
	}

	doc := newDocument()
	doc.plain("new headword")
	doc.selectable("  1. ", "new meaning")
	m = updateModel(t, m, searchResultMsg{id: m.requestID, doc: doc})
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("new results kept old viewport offset: %d", got)
	}
	if !strings.Contains(m.viewport.View(), "new headword") {
		t.Fatal("top of new results is not visible")
	}
}

func TestTUIRenderGolden(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"normal", 80, 24}, {"small", 40, 10}} {
		t.Run(size.name, func(t *testing.T) {
			m := selectionModel()
			m.width, m.height = size.w, size.h
			m.focus = focusResults
			m.visual = true
			m.anchor = 0
			m.pos = 8
			m.thai = thaiPane{open: true, text: "take care of", entries: []model.ThaiEntry{{Thai: "ดูแล", PartOfSpeech: "PHRV"}}}
			m.layout()
			got := []byte(m.View().Content)
			p := filepath.Join("testdata", "tui_"+size.name+".golden")
			if *update {
				if err := os.WriteFile(p, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("render mismatch\ngot:\n%s", got)
			}
		})
	}
}

func TestTUIOnPTY(t *testing.T) {
	old := dbPath
	dbPath = filepath.Join(t.TempDir(), "jisho.db")
	t.Cleanup(func() { dbPath = old })
	if _, err := installThaiArchive(context.Background(), resolveThaiDBPath(), thaiArchive(t, thaiCSV, true)); err != nil {
		t.Fatal(err)
	}
	pt, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer pt.Close()
	defer tty.Close()
	if err := pty.Setsize(pt, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	m := newREPLModel(context.Background(), fakeQuerier{}, "")
	p := tea.NewProgram(m, tea.WithInput(tty), tea.WithOutput(tty))
	done := make(chan error, 1)
	go func() { _, err := p.Run(); done <- err }()
	// Search, browse, find the gloss word, select one character, translate, exit.
	time.Sleep(100 * time.Millisecond)
	_, _ = pt.Write([]byte("alpha\r"))
	time.Sleep(100 * time.Millisecond)
	_, _ = pt.Write([]byte("\t/gloss\rv\x05"))
	time.Sleep(100 * time.Millisecond)
	_, _ = pt.Write([]byte("\x03"))
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("TUI did not exit from PTY")
	}
	_ = tty.Close()
	var captured bytes.Buffer
	buf := make([]byte, 32768)
	for {
		n, err := pt.Read(buf)
		if n > 0 {
			_, _ = captured.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	out := captured.String()
	if !strings.Contains(out, "?1049h") || !strings.Contains(out, "?1049l") || !strings.Contains(out, "jisho") || !strings.Contains(out, "gloss for alpha") || !strings.Contains(out, "EN → TH") {
		t.Fatalf("TUI did not render and restore its screen: %q", out)
	}
}

var _ query.Querier = fakeQuerier{}
