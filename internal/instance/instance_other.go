//go:build !windows

package instance

import "github.com/egoist/mygo"

type Lock struct{}

func Start(open func([]string, string)) (*Lock, bool, error) {
	mygo.App.OnSecondInstance(open)
	return &Lock{}, !mygo.App.RequestSingleInstanceLock(), nil
}

func (*Lock) Close() {}
