package desktop

import (
	"sort"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	languageAuto  = "自动识别"
	languagePlain = "纯文本"
	// Bound tokenization of minified files as well as the visible-line pass.
	maxHighlightLineBytes = 10000
)

var (
	displayLanguages sync.Map
	commonLanguages  = []string{"JSON", "XML", "HTML", "CSS", "JavaScript", "TypeScript", "Go", "Python", "SQL", "YAML", "TOML", "Markdown"}
)

type boundedTokenizer struct{ editor.Tokenizer }

func (t boundedTokenizer) Tokenize(line string, state uint64, tokens []editor.Token) ([]editor.Token, uint64) {
	if len(line) > maxHighlightLineBytes {
		return tokens, 0
	}
	return t.Tokenizer.Tokenize(line, state, tokens)
}

func displayLanguage(base *editor.Language) *editor.Language {
	if base == nil {
		return nil
	}
	if cached, ok := displayLanguages.Load(base); ok {
		return cached.(*editor.Language)
	}
	language := *base
	if language.Tokenizer != nil && language.Highlighter == nil {
		language.Tokenizer = boundedTokenizer{language.Tokenizer}
	}
	cached, _ := displayLanguages.LoadOrStore(base, &language)
	return cached.(*editor.Language)
}

func (a *App) applyLanguage() {
	var language *editor.Language
	switch a.language {
	case "", languageAuto:
		language = editor.LanguageFor(a.doc.Path)
	case languagePlain:
	default:
		language = editor.LanguageNamed(a.language)
	}
	a.doc.Buffer.SetLanguage(displayLanguage(language))
}

func (a *App) setLanguage(name string) {
	a.language = name
	a.applyLanguage()
	a.editor.Focus()
}

func (a *App) languageLabel() string {
	if language := a.doc.Buffer.Language(); language != nil {
		return language.Name
	}
	return languagePlain
}

func (a *App) languageTooltip() string {
	label := "代码格式：" + a.languageLabel()
	if a.language == "" || a.language == languageAuto {
		label += "（自动识别）"
	}
	return label
}

func (a *App) languageMenu(m *ui.Menu) {
	a.jsonFormatMenuItem(m)
	m.Separator()
	for _, name := range []string{languageAuto, languagePlain} {
		if m.Item(name).Radio(a.language == name || name == languageAuto && a.language == "").Chosen() {
			a.setLanguage(name)
		}
	}
	m.Separator()
	for _, name := range commonLanguages {
		if m.Item(name).Radio(a.language == name).Chosen() {
			a.setLanguage(name)
		}
	}
	m.Submenu("其他语言", func(m *ui.Menu) {
		others := make([]string, 0, len(editor.Languages()))
		for _, language := range editor.Languages() {
			common := false
			for _, name := range commonLanguages {
				common = common || language.Name == name
			}
			if !common {
				others = append(others, language.Name)
			}
		}
		sort.Strings(others)
		for _, name := range others {
			if m.Item(name).Radio(a.language == name).Chosen() {
				a.setLanguage(name)
			}
		}
	})
}
