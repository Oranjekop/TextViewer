package desktop

import (
	"errors"

	"github.com/Oranjekop/TextViewer/internal/fileassoc"
	"github.com/Oranjekop/TextViewer/internal/shellmenu"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type settingsDraft struct {
	Theme      string
	AutoUpdate bool
	integrationState
}

func (a *App) chooseSettings() {
	a.settingsDialog, a.settingsLoading, a.settingsReadFailed, a.settingsError = true, true, false, ""
	a.settingsDraft = settingsDraft{Theme: a.settings.Theme, AutoUpdate: a.settings.AutoCheckUpdates}
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
				a.settingsError = err.Error()
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
		ui.Text(c, "设置").FontSize(18).Bold()
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "外观").Width(96)
			ui.Select(c, &a.settingsDraft.Theme, []string{"跟随系统", "浅色", "深色"}).Width(220).Label("外观").Disabled(a.busy != "")
		})
		ui.Checkbox(c, &a.settingsDraft.AutoUpdate, "启动时检查更新").Disabled(a.busy != "")
		ui.Checkbox(c, &a.settingsDraft.Associations, "关联常用文件格式").
			Disabled(a.settingsLoading || a.settingsReadFailed || a.busy != "")
		ui.Checkbox(c, &a.settingsDraft.ContextMenu, "Windows 11 右键菜单").
			Disabled(a.settingsLoading || a.settingsReadFailed || a.busy != "")
		if ui.Button(c, "默认应用设置…").Disabled(a.settingsLoading || a.busy != "" || !a.settingsDraft.Associations).Clicked() {
			if err := fileassoc.OpenDefaults(); err != nil {
				a.settingsError = err.Error()
			}
		}
		if a.settingsLoading {
			ui.Text(c, "正在读取系统设置…").TextColor(c.Theme().TextMuted)
		}
		if a.settingsError != "" {
			ui.Text(c, a.settingsError).Width(360).SingleLine().Tooltip(a.settingsError).TextColor(c.Theme().Danger)
			if a.settingsReadFailed && ui.Button(c, "重试").Clicked() {
				a.chooseSettings()
			}
		}
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "TextViewer v"+a.version).TextColor(c.Theme().TextMuted)
			label := "检查更新…"
			if a.availableUpdate != nil {
				label = "安装更新 v" + a.availableUpdate.Version + "…"
			}
			if ui.Button(c, label).Disabled(a.updateChecking || a.busy != "").Clicked() {
				a.settingsDialog = false
				if a.availableUpdate != nil {
					a.updatePending = true
					a.showUpdateIfReady()
				} else {
					a.checkUpdates(true)
				}
			}
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Disabled(a.busy != "").Clicked() {
				a.settingsDialog = false
				a.pendingFocus = true
				a.showUpdateIfReady()
			}
			if ui.PrimaryButton(c, "保存").Label("保存设置").Disabled(a.settingsLoading || a.settingsReadFailed || a.busy != "").Clicked() {
				a.saveSettings(false)
			}
		})
	}).Width(408)
	if wasOpen && a.busy != "" {
		a.settingsDialog = true
	}
}

func (a *App) saveSettings(trust bool) {
	if a.settingsLoading || a.busy != "" {
		return
	}
	draft := a.settingsDraft
	a.settingsError = ""
	var id uint64
	if a.Win != nil {
		_, id = a.startJob("正在保存设置…")
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
				a.settingsError = err.Error()
				return
			}
			next := a.settings
			next.Theme, next.AutoCheckUpdates = draft.Theme, draft.AutoUpdate
			next.FileAssociations, next.ContextMenu = &draft.Associations, &draft.ContextMenu
			if err := savePreferences(next); err != nil {
				a.settingsError = "设置未保存：" + err.Error()
				return
			}
			a.settings = next
			a.settingsDialog, a.pendingFocus = false, true
			if !a.settings.AutoCheckUpdates {
				a.stopUpdateCheck()
			}
			a.status = "设置已保存"
			if cleanupErr != nil {
				a.status += "；旧版清理未完成：" + cleanupErr.Error()
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
	result, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent: a.Win, Type: mygo.MessageQuestion, Title: "添加新版右键菜单",
		Message: "Windows 需要信任 TextViewer 的本机签名",
		Detail:  "继续后会出现管理员确认窗口。仅信任 TextViewer 的签名，不会开启开发者模式，也不会更改 TXT 默认打开方式。",
		Buttons: []string{"继续", "取消"}, DefaultButton: 1, CancelButton: 1,
	})
	if err == nil && result.Button == 0 {
		a.saveSettings(true)
	} else {
		a.settingsError = "右键菜单未启用，设置尚未保存"
	}
}
