package desktop

import (
	"github.com/Oranjekop/TextViewer/internal/filetypes"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

var associationOptions = filetypes.AssociationOptions()

func (a *App) chooseAssociations() {
	a.associationDraft = make(map[string]bool)
	groups, err := filetypes.SelectedAssociationGroups(a.settingsDraft.extensions)
	if err != nil {
		groups = filetypes.AssociationGroups()
	}
	for _, group := range groups {
		for _, extension := range group.Extensions {
			a.associationDraft[extension] = true
		}
	}
	a.associationScroll = ui.ScrollState{}
	a.associationDialog = true
}

func (a *App) associationChooser(c *ui.Context) {
	ui.Modal(c.Key("association-dialog"), &a.associationDialog, func() {
		ui.Text(c, locales.ChooseExtensions()).FontSize(18).Bold()
		ui.Scroll(c.Key("extension-list")).Height(200).FillWidth().TrackScroll(&a.associationScroll).Children(func() {
			ui.Column(c).Gap(8).FillWidth().Children(func() {
				for _, group := range associationOptions {
					var label string
					switch group.ID {
					case "text":
						label = locales.ExtensionText()
					case "markdown":
						label = "Markdown"
					case "code":
						label = locales.ExtensionCode()
					case "data":
						label = locales.ExtensionData()
					case "config":
						label = locales.ExtensionConfig()
					}
					ui.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted)
					ui.Grid(c.Key(group.ID)).Columns(3).Gap(6).FillWidth().Children(func() {
						for _, extension := range group.Extensions {
							selected := a.associationDraft[extension]
							if ui.Checkbox(c.Key(extension), &selected, extension).Changed() {
								a.associationDraft[extension] = selected
							}
						}
					})
				}
			})
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.DefaultExtensions()).Clicked() {
				a.associationDraft = make(map[string]bool)
				for _, group := range filetypes.AssociationGroups() {
					for _, extension := range group.Extensions {
						a.associationDraft[extension] = true
					}
				}
			}
			if ui.Button(c, locales.Cancel()).Clicked() {
				a.associationDialog = false
			}
			hasSelection := false
			for _, selected := range a.associationDraft {
				hasSelection = hasSelection || selected
			}
			if ui.PrimaryButton(c, locales.Apply()).Disabled(!hasSelection).Clicked() {
				var extensions []string
				for _, group := range associationOptions {
					for _, extension := range group.Extensions {
						if a.associationDraft[extension] {
							extensions = append(extensions, extension)
						}
					}
				}
				a.settingsDraft.extensions = extensions
				a.associationDialog = false
			}
		})
	}).Width(408).Gap(12)
}
