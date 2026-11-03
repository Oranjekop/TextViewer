package desktop

import (
	"errors"

	"github.com/Oranjekop/TextViewer/internal/fileassoc"
	"github.com/Oranjekop/TextViewer/internal/shellmenu"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type settingsDraft struct {
	Theme      string
	Locale     string
	AutoUpdate bool
	integrationState
}

func (a *App) chooseSettings() {
	a.settingsDialog, a.settingsLoading, a.settingsReadFailed, a.settingsError = true, true, false, nil
	if !a.updateChecking {
		a.updateMessage, a.updateFailed = nil, false
	}
	a.settingsDraft = settingsDraft{Theme: a.settings.Theme, Locale: a.settings.Locale, AutoUpdate: a.settings.AutoCheckUpdates}
	a.settingsLoadID++
	id := a.settingsLoadID
	work := func() {
		state, err := a.integration.State()
		done := func() {
			if a.closed || !a.settingsDialog || a.settingsLoadID != id {
				return
			}
			a.settingsLoading = false
			if err != nil {
				a.settingsError = err.Error
				a.settingsReadFailed = true
			} else {
				a.settingsDraft.integrationState = state
			}
		}
		if a.Win == nil {
			done()
		} else {
			a.Win.Update(done)
		}
	}
	if a.Win == nil {
		work()
	} else {
		go work()
	}
}

func (a *App) settingsWindow(c *ui.Context) {
	wasOpen := a.settingsDialog
	ui.Modal(c.Key("settings-dialog"), &a.settingsDialog, func() {
		ui.Text(c, locales.Settings()).FontSize(18).Bold()
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, locales.Appearance()).Width(96)
			ui.MenuButton(c, themeLabel(a.settingsDraft.Theme), func(m *ui.Menu) {
				for _, theme := range []string{"跟随系统", "浅色", "深色"} {
					if m.Item(themeLabel(theme)).Radio(a.settingsDraft.Theme == theme).Chosen() {
						a.settingsDraft.Theme = theme
					}
				}
			}).Width(220).Label(locales.Appearance()).Disabled(a.busy != nil)
		})
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, locales.InterfaceLanguage()).Width(96)
			ui.MenuButton(c, localeLabel(a.settingsDraft.Locale), func(m *ui.Menu) {
				for _, locale := range []string{"", "zh-Hans", "en"} {
					if m.Item(localeLabel(locale)).Radio(a.settingsDraft.Locale == locale).Chosen() {
						a.settingsDraft.Locale = locale
					}
				}
			}).Width(220).Label(locales.InterfaceLanguage()).Disabled(a.busy != nil)
		})
		ui.Checkbox(c, &a.settingsDraft.AutoUpdate, locales.AutoUpdate()).Disabled(a.busy != nil)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Checkbox(c, &a.settingsDraft.Associations, locales.FileAssociations()).
				Grow(1).Disabled(a.settingsLoading || a.settingsReadFailed || a.busy != nil)
			if ui.Button(c, locales.DefaultApps()).Disabled(a.settingsLoading || a.busy != nil || !a.settingsDraft.Associations).Clicked() {
				if err := fileassoc.OpenDefaults(); err != nil {
					a.settingsError = err.Error
				}
			}
		})
		ui.Checkbox(c, &a.settingsDraft.ContextMenu, locales.ContextMenu()).
			Disabled(a.settingsLoading || a.settingsReadFailed || a.busy != nil)
		if a.settingsLoading {
			ui.Text(c, locales.ReadingSettings()).TextColor(c.Theme().TextMuted)
		}
		if a.settingsError != nil {
			text := a.settingsError.String()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, text).Grow(1).MinWidth(0).SingleLine().Tooltip(text).TextColor(c.Theme().Danger)
				if a.settingsReadFailed && ui.Button(c, locales.Retry()).Clicked() {
					a.chooseSettings()
				}
			})
		}
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "TextViewer v"+a.version).TextColor(c.Theme().TextMuted)
			if a.updateChecking {
				ui.Spinner(c).Size(14, 14).Label(locales.CheckingUpdatesLabel())
			}
			label := locales.CheckUpdates()
			if a.updateChecking {
				label = locales.CheckingUpdates()
			} else if a.availableUpdate != nil {
				label = locales.InstallVersion(a.availableUpdate.Version)
			}
			if ui.Button(c.Key("check-update"), label).Disabled(a.updateChecking || a.busy != nil).Clicked() {
				if a.availableUpdate != nil {
					a.settingsDialog = false
					a.updatePending = true
					a.showUpdateIfReady()
				} else {
					a.checkUpdates(true)
				}
			}
		})
		if a.updateMessage != nil {
			color := c.Theme().TextMuted
			if a.updateFailed {
				color = c.Theme().Danger
			}
			text := a.updateMessage.String()
			ui.Text(c, text).Width(360).SingleLine().Tooltip(text).FontSize(12).TextColor(color)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.Cancel()).Disabled(a.busy != nil).Clicked() {
				a.settingsDialog = false
				a.pendingFocus = true
				a.showUpdateIfReady()
			}
			if ui.PrimaryButton(c, locales.Save()).Label(locales.SaveSettings()).Disabled(a.settingsLoading || a.settingsReadFailed || a.busy != nil).Clicked() {
				a.saveSettings(false)
			}
		})
	}).Width(408).Gap(8)
	if wasOpen && a.busy != nil {
		a.settingsDialog = true
	}
}

func (a *App) saveSettings(trust bool) {
	if a.settingsLoading || a.busy != nil {
		return
	}
	draft := a.settingsDraft
	a.settingsError = nil
	var id uint64
	if a.Win != nil {
		_, id = a.startJob(locales.SavingSettings)
		a.cancellable = false
	}
	work := func() {
		current, err := a.integration.State()
		if err == nil {
			err = applyIntegrations(a.integration, draft.integrationState, current, trust)
		}
		var cleanupErr error
		if err == nil {
			cleanupErr = a.integration.Cleanup()
		}
		done := func() {
			if errors.Is(err, shellmenu.ErrTrustRequired) {
				a.confirmSettingsTrust()
				return
			}
			if err != nil {
				a.settingsError = err.Error
				return
			}
			next := a.settings
			next.Theme, next.AutoCheckUpdates = draft.Theme, draft.AutoUpdate
			next.Locale = normalizeLocale(draft.Locale)
			next.FileAssociations, next.ContextMenu = &draft.Associations, &draft.ContextMenu
			if err := savePreferences(next); err != nil {
				a.settingsError = locales.WrapError(err, locales.SettingsNotSaved).Error
				return
			}
			a.settings = next
			if a.setLocale != nil {
				a.setLocale(next.Locale)
			}
			a.settingsDialog, a.pendingFocus = false, true
			if !a.settings.AutoCheckUpdates {
				a.stopUpdateCheck()
			}
			a.status = locales.SettingsSaved
			if cleanupErr != nil {
				a.status = locales.WrapError(cleanupErr, locales.CleanupIncomplete).Error
			}
		}
		if a.Win == nil {
			done()
		} else {
			a.finishJob(id, done)
		}
	}
	if a.Win == nil {
		work()
	} else {
		go work()
	}
}

func (a *App) confirmSettingsTrust() {
	result, err := a.dialogs.Message(mygo.MessageOptions{
		Parent: a.Win, Type: mygo.MessageQuestion, Title: locales.ContextMenuTitle(),
		Message: locales.TrustRequired(),
		Detail:  locales.TrustDetail(),
		Buttons: []string{locales.ContinueAction(), locales.Cancel()}, DefaultButton: 1, CancelButton: 1,
	})
	if err == nil && result.Button == 0 {
		a.saveSettings(true)
	} else {
		a.settingsError = locales.ContextMenuNotSaved
	}
}
