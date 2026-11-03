package locales

// Message keeps transient UI text in the current language, including messages
// produced by a background task before the user changes languages.
type Message func() string

func (m Message) String() string {
	if m == nil {
		return ""
	}
	return m()
}

func Text(text string) Message { return func() string { return text } }

type localizedError struct {
	message Message
	cause   error
}

func (e *localizedError) Error() string { return e.message.String() }
func (e *localizedError) Unwrap() error { return e.cause }

func NewError(message Message) error { return &localizedError{message: message} }

// WrapError preserves errors.Is/As and formats the cause only when displayed.
func WrapError(cause error, format func(string) string) error {
	return &localizedError{
		message: func() string { return format(cause.Error()) },
		cause:   cause,
	}
}
