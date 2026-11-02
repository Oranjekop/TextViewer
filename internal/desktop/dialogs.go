package desktop

import "github.com/egoist/mygo"

type dialogService interface {
	Open(mygo.OpenDialogOptions) ([]string, error)
	Save(mygo.SaveDialogOptions) (string, error)
	Message(mygo.MessageOptions) (mygo.MessageResult, error)
}
