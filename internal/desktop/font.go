package desktop

import (
	"sync"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

const fontAutomatic = "@automatic"

var installedFonts = sync.OnceValue(func() []string {
	families := ui.FontFamilies()
	names := make([]string, 1, len(families)+1)
	names[0] = fontAutomatic
	for _, family := range families {
		names = append(names, family.Name)
	}
	return names
})

func (a *App) editorFont() string {
	if a.settings.FontFamily != "" {
		return a.settings.FontFamily
	}
	if a.doc.Buffer.Language() != nil {
		return "monospace"
	}
	return defaultTextFont()
}

func (a *App) chooseFont() {
	a.fontDraft = a.settings.FontFamily
	if a.fontDraft == "" {
		a.fontDraft = fontAutomatic
	}
	a.refreshFontOptions()
	a.fontDialog = true
}

func (a *App) refreshFontOptions() {
	// Cache only actual system families; translated labels are per dialog.
	a.fontOptions = append([]string{locales.Automatic()}, installedFonts()[1:]...)
	a.fontInput = a.fontDraft
	if a.fontDraft == fontAutomatic {
		a.fontInput = locales.Automatic()
	}
}

func (a *App) applyFont() {
	a.settings.FontFamily = a.fontDraft
	if a.fontDraft == fontAutomatic {
		a.settings.FontFamily = ""
	}
	a.persistPreferences()
	a.fontDialog = false
	a.pendingFocus = true
}

func (a *App) fontChooser(c *ui.Context) {
	ui.Modal(c.Key("font-dialog"), &a.fontDialog, func() {
		ui.Text(c, locales.Font()).FontSize(18).Bold()
		if ui.Combobox(c, &a.fontInput, a.fontOptions).Width(360).Label(locales.FontName()).Changed() {
			a.fontDraft = a.fontInput
			if a.fontInput == locales.Automatic() {
				a.fontDraft = fontAutomatic
			}
		}
		family := a.fontDraft
		if family == fontAutomatic {
			if a.doc.Buffer.Language() == nil {
				family = defaultTextFont()
			} else {
				family = "monospace"
			}
		}
		ui.Text(c, locales.FontPreview()).
			Font(family).FontSize(18).Width(360).Padding(12, 0)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.Cancel()).Clicked() {
				a.fontDialog = false
				a.pendingFocus = true
			}
			if ui.PrimaryButton(c, locales.Apply()).Clicked() {
				a.applyFont()
			}
		})
	}).Width(408)
}
