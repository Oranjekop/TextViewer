package desktop

import "github.com/egoist/mygo"

type fakeDialogs struct {
	open     func(mygo.OpenDialogOptions) ([]string, error)
	save     func(mygo.SaveDialogOptions) (string, error)
	message  func(mygo.MessageOptions) (mygo.MessageResult, error)
	messages []mygo.MessageOptions
}

func (f *fakeDialogs) Open(options mygo.OpenDialogOptions) ([]string, error) {
	if f.open != nil {
		return f.open(options)
	}
	return nil, nil
}

func (f *fakeDialogs) Save(options mygo.SaveDialogOptions) (string, error) {
	if f.save != nil {
		return f.save(options)
	}
	return "", nil
}

func (f *fakeDialogs) Message(options mygo.MessageOptions) (mygo.MessageResult, error) {
	f.messages = append(f.messages, options)
	if f.message != nil {
		return f.message(options)
	}
	return mygo.MessageResult{Button: options.CancelButton}, nil
}
