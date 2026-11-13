//go:build !windows

package startuptrace

import "time"

func beforeMain(time.Time) time.Duration { return 0 }
