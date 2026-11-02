//go:build !windows

package fileassoc

import "errors"

func Register() error           { return errors.New("文件关联需要 Windows") }
func Unregister() error         { return errors.New("文件关联需要 Windows") }
func OpenDefaults() error       { return errors.New("默认应用设置需要 Windows") }
func Registered() (bool, error) { return false, nil }
func RemoveRegistration() error { return Unregister() }
