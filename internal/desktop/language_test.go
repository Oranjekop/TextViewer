package desktop

import (
	"strings"
	"testing"
	"time"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestLanguageMatchesFilenameOnly(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{"", languagePlain}, {"notes.txt", languagePlain}, {"server.log", languagePlain},
		{"DATA.JSON", "JSON"}, {"settings.yaml", "YAML"}, {"main.go", "Go"},
		{"script.py", "Python"}, {"query.sql", "SQL"}, {"index.html", "HTML"},
		{"component.tsx", "TypeScript"}, {"icon.svg", "XML"},
		{"Makefile", "Makefile"}, {"Dockerfile", "Dockerfile"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			a := testApp()
			a.doc.Path = tc.path
			a.doc.Buffer.Reset(`{"code": "func main() {}", "number": 42}`)
			before := a.doc.Buffer.Snapshot()
			a.resetEditor()
			if got := a.languageLabel(); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
			if a.doc.Buffer.Snapshot() != before {
				t.Fatal("language detection changed the text, version or line endings")
			}
		})
	}
}

func TestManualLanguageMenuPreservesDocument(t *testing.T) {
	a := testApp()
	a.doc.Path = "notes.txt"
	a.doc.Buffer.Reset("{\n  \"message\": \"中文\",\n  \"count\": 42\n}\n")
	a.resetEditor()
	before := a.doc.Buffer.Snapshot()
	tester := ui.NewTester(a.view, 760, 600)
	for _, tc := range []struct{ choice, want string }{
		{"JSON", "JSON"}, {languagePlain, languagePlain}, {"Go", "Go"}, {languageAuto, languagePlain},
	} {
		if err := tester.Click("代码格式"); err != nil {
			t.Fatal(err)
		}
		if err := tester.ChooseMenuItem(tc.choice); err != nil {
			t.Fatal(err)
		}
		if got := a.languageLabel(); got != tc.want {
			t.Fatalf("after %s got %s, want %s", tc.choice, got, tc.want)
		}
		if a.doc.Buffer.Snapshot() != before || a.doc.Buffer.CanUndo() {
			t.Fatal("display format edited the document or its undo history")
		}
		code := tc.want != languagePlain
		if (a.state.Font.Family == "monospace") != code || a.state.NoIndentGuides == code {
			t.Fatal("code display did not update the font and indentation guides")
		}
	}
}

func TestLanguageChangesOnSaveAsButKeepsManualChoice(t *testing.T) {
	a := testApp()
	a.doc.Path = "data.json"
	a.applyLanguage()
	if a.languageLabel() != "JSON" {
		t.Fatal("auto format not updated after a filename change")
	}
	a.setLanguage("XML")
	a.doc.Path = "other.py"
	a.applyLanguage()
	if a.languageLabel() != "XML" {
		t.Fatal("save as discarded the manual format")
	}
	a.doc = document.New()
	a.resetEditor()
	if a.languageLabel() != languagePlain || a.language != languageAuto {
		t.Fatal("new file inherited the previous document's manual format")
	}
}

func TestLanguageStaysStableAcrossFrames(t *testing.T) {
	a := testApp()
	a.doc.Path = "main.go"
	a.doc.Buffer.Reset("package main\nfunc main() {}\n")
	a.resetEditor()
	language := a.doc.Buffer.Language()
	tester := ui.NewTester(a.view, 760, 600)
	for range 5 {
		tester.Frame()
		a.applyLanguage()
		if a.doc.Buffer.Language() != language {
			t.Fatal("language recreated and discarded the incremental highlighter")
		}
	}
}

func TestBoundedTokenizerSkipsVeryLongLines(t *testing.T) {
	calls := 0
	tokenizer := boundedTokenizer{editor.TokenizerFunc(func(line string, state uint64, tokens []editor.Token) ([]editor.Token, uint64) {
		calls++
		return append(tokens, editor.Token{End: len(line), Kind: editor.TokenKeyword}), state + 1
	})}
	long := strings.Repeat("x", maxHighlightLineBytes+1)
	if tokens, state := tokenizer.Tokenize(long, 12, nil); len(tokens) != 0 || state != 0 || calls != 0 {
		t.Fatal("long line reached the expensive tokenizer")
	}
	if allocations := testing.AllocsPerRun(10, func() { tokenizer.Tokenize(long, 0, nil) }); allocations != 0 {
		t.Fatalf("skipping long lines allocated %v times", allocations)
	}
	if tokens, state := tokenizer.Tokenize("func", 2, nil); len(tokens) != 1 || state != 3 || calls != 1 {
		t.Fatal("normal code line was not tokenized")
	}
}

func TestMultilineHighlightingAndEdits(t *testing.T) {
	a := testApp()
	a.doc.Path = "main.go"
	a.doc.Buffer.Reset("/* comment\nstill a comment\n*/\nvar answer = 42\n")
	a.resetEditor()
	tokenizer := a.doc.Buffer.Language().Tokenizer
	snapshot := a.doc.Buffer.Snapshot()
	highlighter := editor.NewTokenHighlighter(tokenizer, snapshot)
	if !highlighter.Highlight(snapshot.LineCount()-1, time.Second) {
		t.Fatal("small code fixture did not finish highlighting")
	}
	for _, tc := range []struct {
		line int
		kind editor.TokenKind
	}{{1, editor.TokenComment}, {3, editor.TokenKeyword}, {3, editor.TokenNumber}} {
		tokens, ready := highlighter.Tokens(tc.line, snapshot.Line(tc.line), nil)
		found := false
		for _, token := range tokens {
			found = found || token.Kind == tc.kind
		}
		if !ready || !found {
			t.Fatalf("line %d missing token kind %d", tc.line, tc.kind)
		}
	}
	before := snapshot
	a.doc.Buffer.Edit(editor.Edit{Start: 0, End: 2, Text: "//"})
	snapshot = a.doc.Buffer.Snapshot()
	highlighter.Edit(before, snapshot, []editor.Change{{OldStart: 0, OldEnd: 2, NewStart: 0, NewEnd: 2}})
	if !highlighter.Highlight(snapshot.LineCount()-1, time.Second) {
		t.Fatal("small edit did not finish highlighting")
	}
	tokens, ready := highlighter.Tokens(1, snapshot.Line(1), nil)
	for _, token := range tokens {
		if token.Kind == editor.TokenComment {
			t.Fatal("removing the block-comment opener left stale comment highlighting")
		}
	}
	if !ready {
		t.Fatal("edited line not ready")
	}
}
