package desktop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Oranjekop/TextViewer/internal/releaseinfo"
	"github.com/Oranjekop/TextViewer/internal/update"
	"github.com/egoist/mygo"
)

func (a *App) stopUpdateCheck() {
	if a.updateCancel != nil {
		a.updateCancel()
		a.updateCancel = nil
	}
}

func (a *App) scheduleUpdateCheck() {
	elapsed := time.Since(time.Unix(a.settings.LastUpdateCheck, 0))
	if !a.settings.AutoCheckUpdates || (elapsed >= 0 && elapsed < 24*time.Hour) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCancel = cancel
	go func() {
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			a.Win.Update(func() {
				if !a.closed && ctx.Err() == nil {
					a.checkUpdates(false)
				}
			})
		}
	}()
}

func (a *App) checkUpdates(manual bool) {
	if a.closed || a.updateChecking {
		return
	}
	if _, err := releaseinfo.ParseVersion(a.version); err != nil {
		return
	}
	a.stopUpdateCheck()
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCancel, a.updateChecking = cancel, true
	a.settings.LastUpdateCheck = time.Now().Unix()
	a.persistPreferences()
	go func() {
		release, err := a.updateClient.Check(ctx, a.version)
		a.Win.Update(func() {
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
					a.updateError("暂时无法检查更新，请稍后再试。")
				}
				return
			}
			if release == nil {
				if manual {
					a.status = "当前已是最新版本 v" + a.version
				}
				return
			}
			a.availableUpdate, a.updatePending = release, true
			a.showUpdateIfReady()
		})
	}()
}

func (a *App) showUpdateIfReady() {
	if a.closed || a.Win == nil || a.busy != "" || !a.updatePending || a.availableUpdate == nil {
		return
	}
	a.updatePending = false
	detail := "当前版本 v" + a.version
	if notes := a.availableUpdate.Notes; notes != "" {
		runes := []rune(notes)
		if len(runes) > 1200 {
			runes = runes[:1200]
		}
		detail += "\n\n" + string(runes)
	}
	result, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent: a.Win, Type: mygo.MessageQuestion, Title: "TextViewer 更新",
		Message: "发现新版本 v" + a.availableUpdate.Version, Detail: detail,
		Buttons: []string{"下载安装", "稍后"}, DefaultButton: 1, CancelButton: 1,
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
	if release == nil || a.busy != "" {
		return
	}
	ctx, id := a.startJob("正在下载更新…")
	go func() {
		installer, err := a.updateClient.Download(ctx, release, func(done, total int64) {
			a.Win.Update(func() {
				if !a.closed && a.job == id && a.busy != "" {
					a.busy = fmt.Sprintf("正在下载更新… %d%%", min(100, done*100/total))
				}
			})
		})
		a.finishJob(id, func() {
			if err != nil {
				if errors.Is(err, context.Canceled) {
					a.status = "已取消下载"
				} else {
					a.updateError("更新没有下载完成，请稍后重试。")
				}
				return
			}
			a.updateInstaller = installer
			a.status = "更新已下载，准备安装"
			a.installUpdate()
		})
	}()
}

func (a *App) installUpdate() {
	a.confirmLeave(func() {
		if err := update.StartInstaller(a.updateInstaller); err != nil {
			a.updateInstaller = nil
			a.updateError("安装程序未能启动，请重新下载后再试。")
			return
		}
		a.Win.Destroy()
	})
}

func (a *App) updateError(message string) {
	a.status = message
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{
		Parent: a.Win, Type: mygo.MessageError, Title: "TextViewer 更新",
		Message: message, Buttons: []string{"确定"},
	})
}
