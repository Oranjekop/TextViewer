package desktop

import (
	"context"
	"errors"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestJSONFormattingIsOneUndoableEditAndRetainsOutputSettings(t *testing.T) {
	a := testApp()
	const input = `{"z":9007199254740993,"z":1.00e+20,"text":"中文 \u4e2d"}` + "\n"
	a.doc.Buffer.Set(input)
	a.doc.Encoding = document.UTF16BE
	a.doc.Buffer.SetLineEnding("\r\n")
	a.doc.Buffer.Edit(editor.Edit{Start: 0, Text: " "})
	before := a.doc.Buffer.Snapshot()
	output, changed, err := document.FormatJSON(context.Background(), before)
	if err != nil || !changed {
		t.Fatalf("format: changed=%v err=%v", changed, err)
	}
	if err = applyJSONFormat(context.Background(), a.doc, before, output); err != nil {
		t.Fatal(err)
	}
	if a.doc.Buffer.String() != output || a.doc.Encoding != document.UTF16BE || a.doc.Buffer.LineEnding() != "\r\n" {
		t.Fatal("format changed output settings or failed to apply")
	}
	if !a.doc.Buffer.Undo() || a.doc.Buffer.String() != before.String() {
		t.Fatal("one undo did not restore the original text")
	}
	if !a.doc.Buffer.Undo() || a.doc.Buffer.String() != input {
		t.Fatal("format lost existing undo history")
	}
	a.doc.Buffer.Redo()
	a.doc.Buffer.Redo()
	if a.doc.Buffer.String() != output {
		t.Fatal("redo did not restore formatting")
	}
}

func TestJSONCommitRejectsCancellationAndStaleSnapshot(t *testing.T) {
	for _, change := range []string{"cancel", "text", "line ending"} {
		t.Run(change, func(t *testing.T) {
			a := testApp()
			a.doc.Buffer.Set(`{"a":1}`)
			before := a.doc.Buffer.Snapshot()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch change {
			case "cancel":
				cancel()
			case "text":
				a.doc.Buffer.Set(`{"b":2}`)
			case "line ending":
				a.doc.Buffer.SetLineEnding("\n")
			}
			current := a.doc.Buffer.Snapshot()
			err := applyJSONFormat(ctx, a.doc, before, "replacement")
			if err == nil || a.doc.Buffer.Snapshot() != current {
				t.Fatal("stale/canceled result was applied")
			}
			if change == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation: %v", err)
			}
		})
	}
}

func TestJSONActionIsAvailableFromCodeAndEditingMenus(t *testing.T) {
	a := testApp()
	tester := ui.NewTester(a.view, 760, 600)
	for _, open := range []func() error{
		func() error { return tester.Click("代码格式") },
		func() error { return tester.RightClick("文本编辑区") },
	} {
		if err := open(); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, label := range tester.Menu() {
			found = found || label == "格式化 JSON"
		}
		if !found {
			t.Fatalf("format action missing from %v", tester.Menu())
		}
		tester.CloseMenu()
	}
}
