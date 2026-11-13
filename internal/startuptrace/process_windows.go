package startuptrace

import (
	"time"

	"golang.org/x/sys/windows"
)

func beforeMain(entered time.Time) time.Duration {
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user) != nil {
		return 0
	}
	return max(0, entered.Sub(time.Unix(0, created.Nanoseconds())))
}
