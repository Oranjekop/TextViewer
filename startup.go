package main

func fileOpenHandler(whenReady func(func()), open func([]string, string)) func(string) {
	return func(path string) {
		// macOS can deliver Finder's open event before the startup callback
		// creates the window. Keep the path until that callback has run.
		whenReady(func() { open([]string{path}, "") })
	}
}
