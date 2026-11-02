package desktop

import (
	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/egoist/mygo/ui"
)

func (a *App) encodingMenu(m *ui.Menu) {
	for _, encoding := range []document.Encoding{document.UTF8, document.UTF8BOM, document.UTF16LE, document.UTF16BE, document.GB18030} {
		if m.Item(string(encoding)).Radio(a.doc.Encoding == encoding).Disabled(a.busy != "").Chosen() {
			a.doc.Encoding = encoding
			a.status = ""
			a.pendingFocus = true
		}
	}
}

func (a *App) lineEndingLabel() string {
	if a.doc.Buffer.LineEnding() == "\r\n" {
		return "Windows (CRLF)"
	}
	return "Unix / Linux (LF)"
}

func (a *App) lineEndingMenu(m *ui.Menu) {
	for _, option := range []struct{ label, ending string }{
		{"Windows (CRLF)", "\r\n"},
		{"Unix / Linux (LF)", "\n"},
	} {
		if m.Item(option.label).Radio(a.doc.Buffer.LineEnding() == option.ending).Disabled(a.busy != "").Chosen() {
			a.doc.Buffer.SetLineEnding(option.ending)
			a.status = ""
			a.pendingFocus = true
		}
	}
}
