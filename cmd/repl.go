package cmd

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/shsnail/jisho/internal/model"
	"github.com/shsnail/jisho/internal/query"
	"github.com/spf13/cobra"
)

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Start the keyboard-first terminal UI",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runREPLWithQuerier(cmd.Context(), query.New(db))
	},
}

func init() { rootCmd.AddCommand(replCmd) }

type replFocus uint8

const (
	focusPrompt replFocus = iota
	focusResults
	focusFind
)

type replModel struct {
	ctx           context.Context
	q             query.Querier
	prompt        textinput.Model
	find          textinput.Model
	viewport      viewport.Model
	focus         replFocus
	doc           resultDocument
	field, pos    int
	visual        bool
	anchor        int
	matches       []textMatch
	match         int
	width, height int
	loading       bool
	requestID     int
	status        string
	help          bool
	thai          thaiPane
	history       []string
	historyIndex  int
	historyPath   string
}

type textMatch struct{ field, start, end int }
type searchResultMsg struct {
	id  int
	doc resultDocument
	err error
}
type thaiResultMsg struct {
	id      int
	text    string
	entries []model.ThaiEntry
	err     error
	words   bool
}

type thaiPane struct {
	open, details, loading, phraseMiss, wordMode bool
	text                                         string
	entries                                      []model.ThaiEntry
	wordEntries                                  []wordThaiResult
	err                                          error
}
type wordThaiResult struct {
	word    string
	entries []model.ThaiEntry
	err     error
}

var (
	styleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleCursor = lipgloss.NewStyle().Reverse(true)
	styleSelect = lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15"))
	styleMatch  = lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0"))
	styleError  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleThai   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

func newREPLModel(ctx context.Context, q query.Querier, historyPath string) replModel {
	p := textinput.New()
	p.Prompt = "jisho> "
	p.Placeholder = "Japanese, romaji, or English"
	p.ShowSuggestions = true
	p.SetSuggestions([]string{"/search ", "/kanji ", "/name ", "/radical ", "/th ", "/help", "/exit"})
	p.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("right"))
	p.KeyMap.NextSuggestion.SetEnabled(false)
	p.KeyMap.PrevSuggestion.SetEnabled(false)
	p.Focus()
	f := textinput.New()
	f.Prompt = "/"
	f.Placeholder = "find English text"
	f.Blur()
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(18))
	vp.SoftWrap = false
	m := replModel{ctx: ctx, q: q, prompt: p, find: f, viewport: vp, focus: focusPrompt,
		doc:   resultDocument{lines: []resultLine{{text: "Type a query and press Enter. Tab browses results.", field: -1}}},
		field: -1, match: -1, width: 80, height: 24, historyPath: historyPath}
	m.history = loadREPLHistory(historyPath)
	m.historyIndex = len(m.history)
	m.refreshViewport()
	return m
}

func runREPLWithQuerier(ctx context.Context, q query.Querier) error {
	m := newREPLModel(ctx, q, replHistoryPath())
	p := tea.NewProgram(m, tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

func (m replModel) Init() tea.Cmd { return textinput.Blink }

func (m replModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(20, msg.Width), max(8, msg.Height)
		m.layout()
		return m, nil
	case searchResultMsg:
		if msg.id != m.requestID {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.doc, m.status, m.field, m.pos = msg.doc, "", firstField(msg.doc), 0
		m.visual, m.matches, m.match = false, nil, -1
		m.thai = thaiPane{}
		// SetContent preserves the viewport's prior offset. A new result set must
		// start at its first row, or a scroll position from the previous query can
		// hide its headword.
		m.viewport.SetYOffset(0)
		m.refreshViewport()
		return m, nil
	case thaiResultMsg:
		if msg.id != m.requestID {
			return m, nil
		}
		m.thai.loading = false
		if msg.words {
			m.thai.wordMode = true
			return m, nil
		}
		m.thai.err, m.thai.entries = msg.err, msg.entries
		m.thai.phraseMiss = msg.err == nil && len(msg.entries) == 0 && strings.Contains(strings.TrimSpace(msg.text), " ")
		return m, nil
	case wordLookupMsg:
		if msg.id != m.requestID {
			return m, nil
		}
		m.thai.loading, m.thai.wordMode, m.thai.wordEntries = false, true, msg.results
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	var cmd tea.Cmd
	if m.focus == focusPrompt {
		m.prompt, cmd = m.prompt.Update(msg)
	}
	if m.focus == focusFind {
		m.find, cmd = m.find.Update(msg)
	}
	return m, cmd
}

func (m replModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	if m.thai.open {
		switch k {
		case "esc":
			m.thai = thaiPane{}
			return m, nil
		case "d":
			m.thai.details = !m.thai.details
			return m, nil
		case "enter":
			if m.thai.phraseMiss && !m.thai.loading {
				return m.startWordLookup()
			}
		}
	}
	switch m.focus {
	case focusPrompt:
		switch k {
		case "tab":
			if len(m.doc.fields) > 0 {
				m.focus = focusResults
				m.prompt.Blur()
				m.ensureCursor()
				m.refreshViewport()
			}
			return m, nil
		case "enter":
			return m.submitPrompt()
		case "up":
			m.recallHistory(-1)
			return m, nil
		case "down":
			m.recallHistory(1)
			return m, nil
		case "ctrl+d":
			if m.prompt.Value() == "" {
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		return m, cmd
	case focusFind:
		switch k {
		case "esc":
			m.focus = focusResults
			m.find.Blur()
			return m, nil
		case "enter":
			m.applyFind(m.find.Value())
			m.focus = focusResults
			m.find.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.find, cmd = m.find.Update(msg)
		return m, cmd
	case focusResults:
		return m.handleResultKey(k)
	}
	return m, nil
}

func (m replModel) handleResultKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "tab", "esc":
		if m.visual {
			m.visual = false
			m.refreshViewport()
			return m, nil
		}
		m.focus = focusPrompt
		m.prompt.Focus()
		return m, textinput.Blink
	case "/":
		m.focus = focusFind
		m.find.SetValue("")
		m.find.Focus()
		return m, textinput.Blink
	case "n":
		m.cycleMatch(1)
	case "N":
		m.cycleMatch(-1)
	case "v":
		m.visual = !m.visual
		if m.visual {
			m.anchor = m.pos
		}
	case "h", "left":
		m.moveChar(-1)
	case "l", "right":
		m.moveChar(1)
	case "j", "down":
		m.moveField(1)
	case "k", "up":
		m.moveField(-1)
	case "w":
		m.moveWordForward(false)
	case "e":
		m.moveWordForward(true)
	case "b":
		m.moveWordBackward()
	case "ctrl+d":
		m.viewport.HalfPageDown()
	case "ctrl+u":
		m.viewport.HalfPageUp()
	case "ctrl+e":
		return m.startThaiLookup(m.lookupText())
	}
	m.refreshViewport()
	return m, nil
}

func (m replModel) submitPrompt() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.prompt.Value())
	if line == "" {
		return m, nil
	}
	m.addHistory(line)
	m.prompt.SetValue("")
	lower := strings.ToLower(line)
	if lower == "/exit" || lower == "/quit" || lower == "/q" {
		return m, tea.Quit
	}
	if lower == "/help" {
		m.help = !m.help
		return m, nil
	}
	if strings.HasPrefix(lower, "/th ") {
		return m.startThaiLookup(strings.TrimSpace(line[4:]))
	}
	m.requestID++
	id := m.requestID
	m.loading, m.status = true, "Searching…"
	return m, func() tea.Msg {
		doc, err := executeREPLQuery(m.ctx, m.q, line)
		return searchResultMsg{id: id, doc: doc, err: err}
	}
}

func executeREPLQuery(ctx context.Context, q query.Querier, line string) (resultDocument, error) {
	if strings.HasPrefix(line, "/") {
		parts := strings.Fields(line[1:])
		if len(parts) == 0 {
			return newDocument(), nil
		}
		switch strings.ToLower(parts[0]) {
		case "search":
			return executeWordSearch(ctx, q, parts[1:])
		case "name":
			if len(parts) < 2 {
				return newDocument(), fmt.Errorf("usage: /name <query>")
			}
			n, err := q.SearchNames(ctx, strings.Join(parts[1:], " "))
			if err != nil {
				return newDocument(), err
			}
			return documentForWords(nil, n), nil
		case "kanji":
			r := []rune(strings.Join(parts[1:], ""))
			if len(r) != 1 {
				return newDocument(), fmt.Errorf("usage: /kanji <one character>")
			}
			k, err := q.LookupKanji(ctx, string(r))
			if err != nil {
				return newDocument(), err
			}
			return documentForKanji(k), nil
		case "radical":
			if len(parts) < 2 {
				return newDocument(), fmt.Errorf("usage: /radical <radical...>")
			}
			var rs []string
			for _, a := range parts[1:] {
				for _, r := range a {
					rs = append(rs, string(r))
				}
			}
			ks, err := q.FilterKanjiByRadicals(ctx, rs)
			if err != nil {
				return newDocument(), err
			}
			return documentForKanjiList(ks), nil
		default:
			return newDocument(), fmt.Errorf("unknown command %q", "/"+parts[0])
		}
	}
	return executeWordSearch(ctx, q, strings.Fields(line))
}

func executeWordSearch(ctx context.Context, q query.Querier, args []string) (resultDocument, error) {
	if len(args) == 0 {
		return newDocument(), fmt.Errorf("search needs a query")
	}
	var opts query.SearchOpts
	var terms []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--common":
			opts.CommonOnly = true
		case "--jlpt":
			i++
			if i >= len(args) {
				return newDocument(), fmt.Errorf("--jlpt requires n1-n5")
			}
			n, err := parseJLPT(args[i])
			if err != nil {
				return newDocument(), err
			}
			opts.JLPTLevel = n
		default:
			terms = append(terms, args[i])
		}
	}
	if len(terms) == 0 {
		return newDocument(), fmt.Errorf("search needs a query")
	}
	text := strings.Join(terms, " ")
	w, err := q.SearchWords(ctx, text, opts)
	if err != nil {
		return newDocument(), err
	}
	n, err := q.SearchNames(ctx, text)
	if err != nil {
		return newDocument(), err
	}
	return documentForWords(w, n), nil
}

func (m replModel) startThaiLookup(text string) (tea.Model, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" {
		m.status = "No English text selected."
		return m, nil
	}
	m.requestID++
	id := m.requestID
	m.thai = thaiPane{open: true, loading: true, text: text}
	return m, func() tea.Msg {
		e, err := lookupThaiEntries(m.ctx, text)
		return thaiResultMsg{id: id, text: text, entries: e, err: err}
	}
}

type wordLookupMsg struct {
	id      int
	results []wordThaiResult
}

func (m replModel) startWordLookup() (tea.Model, tea.Cmd) {
	m.requestID++
	id, words := m.requestID, splitLookupWords(m.thai.text)
	m.thai.loading = true
	return m, func() tea.Msg {
		out := make([]wordThaiResult, 0, len(words))
		for _, w := range words {
			e, err := lookupThaiEntries(m.ctx, w)
			out = append(out, wordThaiResult{word: w, entries: e, err: err})
		}
		return wordLookupMsg{id: id, results: out}
	}
}

func splitLookupWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '\'' || r == '-')
	})
}

func firstField(d resultDocument) int {
	if len(d.fields) == 0 {
		return -1
	}
	return 0
}

func (m *replModel) ensureCursor() {
	if m.field < 0 && len(m.doc.fields) > 0 {
		m.field = 0
		m.pos = 0
	}
	if m.field >= len(m.doc.fields) {
		m.field = len(m.doc.fields) - 1
	}
	if m.field >= 0 {
		gs := graphemes(m.doc.fields[m.field])
		m.pos = min(max(0, m.pos), max(0, len(gs)-1))
	}
}

func (m *replModel) moveChar(delta int) {
	m.ensureCursor()
	if m.field < 0 {
		return
	}
	n := len(graphemes(m.doc.fields[m.field]))
	m.pos = min(max(0, m.pos+delta), max(0, n-1))
}
func (m *replModel) moveField(delta int) {
	if m.visual {
		return
	}
	m.ensureCursor()
	if m.field < 0 {
		return
	}
	m.field = min(max(0, m.field+delta), len(m.doc.fields)-1)
	m.ensureCursor()
}

func (m *replModel) moveWordForward(toEnd bool) {
	m.ensureCursor()
	if m.field < 0 {
		return
	}
	g := graphemes(m.doc.fields[m.field])
	if len(g) == 0 {
		return
	}
	p := m.pos
	if toEnd {
		c := wordClass(g[p])
		for p+1 < len(g) && wordClass(g[p+1]) == c && c != 0 {
			p++
		}
		for p+1 < len(g) && wordClass(g[p+1]) == 0 {
			p++
		}
		if c == 0 && p+1 < len(g) {
			p++
			c = wordClass(g[p])
			for p+1 < len(g) && wordClass(g[p+1]) == c {
				p++
			}
		}
	} else {
		c := wordClass(g[p])
		for p < len(g) && wordClass(g[p]) == c {
			p++
		}
		for p < len(g) && wordClass(g[p]) == 0 {
			p++
		}
		if p >= len(g) {
			p = len(g) - 1
		}
	}
	m.pos = p
}
func (m *replModel) moveWordBackward() {
	m.ensureCursor()
	if m.field < 0 {
		return
	}
	g := graphemes(m.doc.fields[m.field])
	p := max(0, m.pos-1)
	for p > 0 && wordClass(g[p]) == 0 {
		p--
	}
	c := wordClass(g[p])
	for p > 0 && wordClass(g[p-1]) == c {
		p--
	}
	m.pos = p
}

func (m *replModel) lookupText() string {
	if m.field < 0 || m.field >= len(m.doc.fields) {
		return ""
	}
	g := graphemes(m.doc.fields[m.field])
	if len(g) == 0 {
		return ""
	}
	if m.visual {
		a, b := m.anchor, m.pos
		if a > b {
			a, b = b, a
		}
		return strings.TrimSpace(strings.Join(g[a:b+1], ""))
	}
	if wordClass(g[m.pos]) != 1 {
		return strings.TrimSpace(g[m.pos])
	}
	a, b := m.pos, m.pos
	for a > 0 && wordClass(g[a-1]) == 1 {
		a--
	}
	for b+1 < len(g) && wordClass(g[b+1]) == 1 {
		b++
	}
	return strings.Join(g[a:b+1], "")
}

func (m *replModel) applyFind(term string) {
	m.matches = findMatches(m.doc, term)
	m.match = -1
	if len(m.matches) == 0 {
		m.status = fmt.Sprintf("No match for %q", term)
		return
	}
	m.status = ""
	m.cycleMatch(1)
}
func (m *replModel) cycleMatch(delta int) {
	if len(m.matches) == 0 {
		return
	}
	m.match = (m.match + delta + len(m.matches)) % len(m.matches)
	x := m.matches[m.match]
	m.field, m.pos = x.field, x.start
	m.visual = false
	m.ensureCursor()
	m.refreshViewport()
}

func findMatches(d resultDocument, term string) []textMatch {
	needle := strings.ToLower(strings.TrimSpace(term))
	if needle == "" {
		return nil
	}
	var out []textMatch
	for fi, s := range d.fields {
		gs := graphemes(s)
		for start := 0; start < len(gs); start++ {
			for end := start + 1; end <= len(gs); end++ {
				if strings.ToLower(strings.Join(gs[start:end], "")) == needle {
					out = append(out, textMatch{fi, start, end})
					start = end - 1
					break
				}
				if len([]rune(strings.Join(gs[start:end], ""))) > len([]rune(needle))+2 {
					break
				}
			}
		}
	}
	return out
}

func (m *replModel) recallHistory(delta int) {
	if len(m.history) == 0 {
		return
	}
	m.historyIndex = min(max(0, m.historyIndex+delta), len(m.history))
	if m.historyIndex == len(m.history) {
		m.prompt.SetValue("")
	} else {
		m.prompt.SetValue(m.history[m.historyIndex])
	}
	m.prompt.CursorEnd()
}
func (m *replModel) addHistory(line string) {
	if len(m.history) == 0 || m.history[len(m.history)-1] != line {
		m.history = append(m.history, line)
		appendREPLHistory(m.historyPath, line)
	}
	m.historyIndex = len(m.history)
}

func (m *replModel) layout() {
	m.prompt.SetWidth(max(10, m.width-2))
	m.find.SetWidth(max(10, m.width-2))
	m.viewport.SetWidth(m.width)
	m.refreshViewport()
}

func (m *replModel) refreshViewport() {
	thaiHeight := 0
	if m.thai.open {
		thaiHeight = min(max(4, m.height*2/5), 12)
	}
	helpLines := 1
	if m.help && m.height >= 12 {
		helpLines = 5
	}
	m.viewport.SetHeight(max(1, m.height-thaiHeight-helpLines-2))
	content, cursorRow := m.renderDocument(max(10, m.width))
	m.viewport.SetContent(content)
	if cursorRow >= 0 {
		m.viewport.EnsureVisible(cursorRow, 0, 1)
	}
}

func (m replModel) renderDocument(width int) (string, int) {
	var rows []string
	cursorRow := -1
	for _, line := range m.doc.lines {
		if line.field < 0 {
			rows = append(rows, line.text)
			continue
		}
		gs := graphemes(line.text)
		prefix := line.prefix
		avail := max(1, width-lipgloss.Width(prefix))
		start := 0
		for start < len(gs) {
			used, end := 0, start
			for end < len(gs) {
				w := max(1, lipgloss.Width(gs[end]))
				if end > start && used+w > avail {
					break
				}
				used += w
				end++
			}
			if end == start {
				end++
			}
			var b strings.Builder
			b.WriteString(prefix)
			for i := start; i < end; i++ {
				rendered := gs[i]
				if line.field == m.field {
					if i == m.pos {
						cursorRow = len(rows)
					}
					if m.visual {
						a, z := m.anchor, m.pos
						if a > z {
							a, z = z, a
						}
						if i >= a && i <= z {
							rendered = styleSelect.Render(rendered)
						}
					}
					if !m.visual && i == m.pos {
						rendered = styleCursor.Render(rendered)
					}
				}
				for _, match := range m.matches {
					if match.field == line.field && i >= match.start && i < match.end && !(line.field == m.field && i == m.pos) {
						rendered = styleMatch.Render(rendered)
					}
				}
				b.WriteString(rendered)
			}
			rows = append(rows, b.String())
			start = end
			prefix = strings.Repeat(" ", lipgloss.Width(line.prefix))
			avail = max(1, width-lipgloss.Width(prefix))
		}
	}
	return strings.Join(rows, "\n"), cursorRow
}

func (m replModel) View() tea.View {
	title := "jisho"
	if m.loading {
		title += "  searching…"
	}
	if m.focus == focusResults {
		title += "  [NORMAL]"
	}
	if m.visual {
		title = "jisho  [VISUAL]"
	}
	if m.focus == focusFind {
		title += "  [FIND]"
	}
	parts := []string{styleTitle.Render(title), m.viewport.View()}
	if m.thai.open {
		parts = append(parts, m.renderThai(min(max(4, m.height*2/5), 12)))
	}
	if m.focus == focusFind {
		parts = append(parts, m.find.View())
	} else {
		parts = append(parts, m.prompt.View())
	}
	help := "Enter search • Tab results • Ctrl+C quit"
	if m.focus == focusResults {
		help = "/ find • n/N matches • hjkl/arrows move • v select • Ctrl+E Thai • Esc prompt"
	}
	if m.thai.open {
		help = "Esc close Thai • d details"
		if m.thai.phraseMiss {
			help += " • Enter look up words"
		}
	}
	if m.help && m.height >= 12 {
		help = "Prompt: Enter search  ↑/↓ history  Tab results\nResults: / find  n/N matches  hjkl/arrows move  w/e/b words  Ctrl+D/U page\nVisual: v toggle  motions extend  Ctrl+E Thai\nGlobal: Ctrl+C quit  /help toggle help"
	}
	if m.status != "" {
		help = styleError.Render(m.status) + "  " + help
	}
	parts = append(parts, styleDim.Render(help))
	v := tea.NewView(strings.Join(parts, "\n"))
	v.AltScreen = true
	return v
}

func (m replModel) renderThai(maxLines int) string {
	lines := []string{styleThai.Render("── EN → TH ──  " + m.thai.text)}
	if m.thai.loading {
		return clipLines(append(lines, "Looking up…"), maxLines)
	}
	if m.thai.err != nil {
		return clipLines(append(lines, styleError.Render(m.thai.err.Error())), maxLines)
	}
	if m.thai.wordMode {
		for _, wr := range m.thai.wordEntries {
			if wr.err != nil {
				lines = append(lines, wr.word+": "+wr.err.Error())
				continue
			}
			if len(wr.entries) == 0 {
				lines = append(lines, wr.word+" → no entry")
				continue
			}
			for _, e := range wr.entries {
				lines = append(lines, fmt.Sprintf("%s [%s] → %s", wr.word, e.PartOfSpeech, e.Thai))
			}
		}
		return clipLines(lines, maxLines)
	}
	if len(m.thai.entries) == 0 {
		if m.thai.phraseMiss {
			return clipLines(append(lines, "No phrase entry — Enter: look up individual words."), maxLines)
		}
		return clipLines(append(lines, "No entry."), maxLines)
	}
	for _, e := range m.thai.entries {
		lines = append(lines, fmt.Sprintf("[%s] %s", e.PartOfSpeech, e.Thai))
		if m.thai.details {
			if e.Related != "" {
				lines = append(lines, "  related: "+e.Related)
			}
			if e.Synonyms != "" {
				lines = append(lines, "  synonyms: "+e.Synonyms)
			}
			if e.Antonyms != "" {
				lines = append(lines, "  antonyms: "+e.Antonyms)
			}
		}
	}
	return clipLines(lines, maxLines)
}

func clipLines(lines []string, maxLines int) string {
	if len(lines) > maxLines {
		lines = append(lines[:max(1, maxLines-1)], "…")
	}
	return strings.Join(lines, "\n")
}
