package desktop

import (
	"context"
	"errors"
	"strconv"

	"github.com/Oranjekop/TextViewer/internal/releaseinfo"
	"github.com/Oranjekop/TextViewer/internal/update"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo"
)

type updateService interface {
	Check(context.Context, string) (*update.Release, error)
	Download(context.Context, *update.Release, func(int64, int64)) (*update.Installer, error)
}

func (a *App) stopUpdateCheck() {
	if a.updateCancel != nil {
		a.updateCancel()
		a.updateCancel = nil
	}
}

func (a *App) checkUpdatesOnStartup() {
	if a.settings.AutoCheckUpdates {
		a.checkUpdates(false)
	}
}

func (a *App) checkUpdates(manual bool) {
	if a.closed || a.updateChecking {
		return
	}
	if _, err := releaseinfo.ParseVersion(a.version); err != nil {
		if manual {
			a.updateMessage, a.updateFailed = locales.InvalidUpdateVersion, true
		}
		return
	}
	a.stopUpdateCheck()
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCancel, a.updateChecking = cancel, true
	a.updateMessage, a.updateFailed = nil, false
	a.background(func() {
		release, err := a.updateClient.Check(ctx, a.version)
		a.updateUI(func() {
			if a.closed {
				return
			}
			a.updateChecking = false
			canceled := ctx.Err() != nil
			a.stopUpdateCheck()
			if canceled {
				return
			}
			if err != nil {
				if manual && !errors.Is(err, context.Canceled) {
					a.updateMessage, a.updateFailed = locales.UpdateCheckFailed, true
					if !a.settingsDialog {
						a.updateError(a.updateMessage)
					}
				}
				return
			}
			if release == nil {
				if manual {
					version := a.version
					a.updateMessage = func() string { return locales.UpToDate(version) }
					a.status = a.updateMessage
				}
				return
			}
			a.availableUpdate, a.updatePending = release, true
			if manual && a.settingsDialog {
				a.updatePending = false
			}
			a.showUpdateIfReady()
		})
	})
}

func (a *App) showUpdateIfReady() {
	if a.closed || a.Win == nil || a.busy != nil || a.confirming || a.settingsDialog || a.fontDialog || a.compareDialog || a.comparison != nil || !a.updatePending || a.availableUpdate == nil {
		return
	}
	a.updatePending = false
	detail := locales.CurrentVersion(a.version)
	if notes := a.availableUpdate.Notes; notes != "" {
		runes := []rune(notes)
		if len(runes) > 1200 {
			runes = runes[:1200]
		}
		detail += "\n\n" + string(runes)
	}
	result, err := a.dialogs.Message(mygo.MessageOptions{
		Parent: a.Win, Type: mygo.MessageQuestion, Title: locales.UpdateTitle(),
		Message: locales.NewVersion(a.availableUpdate.Version), Detail: detail,
		Buttons: []string{locales.DownloadInstall(), locales.Later()}, DefaultButton: 1, CancelButton: 1,
	})
	if err == nil && result.Button == 0 {
		a.downloadUpdate()
	}
}

func (a *App) downloadUpdate() {
	if a.updateInstaller != nil {
		a.installUpdate()
		return
	}
	release := a.availableUpdate
	if release == nil || a.busy != nil {
		return
	}
	ctx, id := a.startJob(locales.DownloadingUpdate)
	go func() {
		installer, err := a.updateClient.Download(ctx, release, func(done, total int64) {
			a.Win.Update(func() {
				if !a.closed && a.job == id && a.busy != nil {
					percent := strconv.FormatInt(min(100, done*100/total), 10)
					a.busy = func() string { return locales.DownloadProgress(percent) }
				}
			})
		})
		a.finishJob(id, func() {
			if err != nil {
				if errors.Is(err, context.Canceled) {
					a.status = locales.DownloadCanceled
				} else {
					a.updateError(locales.DownloadFailed)
				}
				return
			}
			a.updateInstaller = installer
			a.status = locales.UpdateReady
			a.installUpdate()
		})
	}()
}

func (a *App) installUpdate() {
	a.confirmAllTabs(func() {
		if err := update.StartInstaller(a.updateInstaller); err != nil {
			a.updateInstaller = nil
			a.updateError(locales.InstallerFailed)
			return
		}
		a.Win.Destroy()
	})
}

func (a *App) updateError(message locales.Message) {
	a.status = message
	_, _ = a.dialogs.Message(mygo.MessageOptions{
		Parent: a.Win, Type: mygo.MessageError, Title: locales.UpdateTitle(),
		Message: message.String(), Buttons: []string{locales.Ok()},
	})
}
