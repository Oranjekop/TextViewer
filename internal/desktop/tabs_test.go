package desktop

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func desktopFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenMultipleFilesPreservesExistingEditsAndReusesOpenTabs(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Set("unsaved text")
	original := a.documentTab
	first := desktopFile(t, "first.json", "{\"first\":1}\n")
	second := desktopFile(t, "second.txt", "second\r\n")
	a.dialogs.(*fakeDialogs).open = func(options mygo.OpenDialogOptions) ([]string, error) {
		if !options.Multiple {
			t.Fatal("file picker does not allow selecting multiple files")
		}
		return []string{first, second, first}, nil
	}
	a.chooseOpen()
	if len(a.tabs) != 3 || !document.SamePath(a.doc.Path, first) || a.busy != nil || len(a.pendingOpen) != 0 {
		t.Fatalf("opened %d tabs, current %q, busy %q, queue %v", len(a.tabs), a.doc.Path, a.busy, a.pendingOpen)
	}
	if original.doc.Buffer.String() != "unsaved text" || len(a.dialogs.(*fakeDialogs).messages) != 0 {
		t.Fatal("opening another file changed or asked to replace the existing document")
	}
	a.doc.Buffer.Set("edited JSON")
	state, snapshot := a.state, a.doc.Buffer.Snapshot()
	a.open(second)
	a.open(first)
	if a.state != state || a.doc.Buffer.Snapshot() != snapshot || len(a.tabs) != 3 {
		t.Fatal("reopening a file reloaded it or lost its edits/editor state")
	}
}

func TestQueuedFilesAllOpenAndBadFileDoesNotDropFollowingPaths(t *testing.T) {
	a := testApp()
	first := desktopFile(t, "first.txt", "first")
	second := desktopFile(t, "second.txt", "second")
	_, id := a.startJob(locales.Saving)
	a.openFiles([]string{first, first + ".missing", second})
	if len(a.tabs) != 1 || len(a.pendingOpen) != 3 {
		t.Fatal("opening while busy did not queue all paths")
	}
	a.finishJob(id, func() {})
	if len(a.tabs) != 2 || !document.SamePath(a.tabs[0].doc.Path, first) || !document.SamePath(a.doc.Path, second) {
		t.Fatalf("queued files missing after a load error: %d tabs, current %q", len(a.tabs), a.doc.Path)
	}
	if len(a.dialogs.(*fakeDialogs).messages) != 1 {
		t.Fatal("failed file was not reported once")
	}
}

func TestOpeningFirstFileClearsEmptyTabSearchResult(t *testing.T) {
	a := testApp()
	a.searchStatus = locales.Text("未找到")
	a.open(desktopFile(t, "first.txt", "first file"))
	if len(a.tabs) != 1 || a.searchStatus != nil {
		t.Fatal("reusing the initial empty tab kept its stale search result")
	}
}

func TestTabSwitchPreservesCursorScrollUndoAndLanguage(t *testing.T) {
	a := testApp()
	a.doc.Path = "first.json"
	a.doc.Buffer.Set(strings.Repeat("first line\n", 500))
	a.setLanguage("JSON")
	first := a.documentTab
	tester := ui.NewTester(a.view, 760, 600)
	first.state.Select(1000, 1000)
	first.state.ScrollTo(80)
	tester.Frame()
	firstCursor, firstScroll := first.state.Cursor(), first.state.ScrollY()
	firstSnapshot := first.doc.Buffer.Snapshot()
	tester.Key(ui.Ctrl, ui.KeyN)
	second := a.documentTab
	if second == first || len(a.tabs) != 2 {
		t.Fatal("Ctrl+N did not create another tab")
	}
	tester.Type("second document")
	tester.Key(ui.Ctrl|ui.Shift, ui.KeyTab)
	if a.documentTab != first || a.state.Cursor() != firstCursor || a.state.ScrollY() != firstScroll ||
		a.language != "JSON" || a.doc.Buffer.Snapshot() != firstSnapshot {
		t.Fatalf("first tab state changed: cursor %d/%d, scroll %f/%f, language %s",
			a.state.Cursor(), firstCursor, a.state.ScrollY(), firstScroll, a.language)
	}
	if err := tester.Click("标签：" + second.name()); err != nil {
		t.Fatal(err)
	}
	tester.Key(ui.Ctrl, ui.KeyZ)
	if a.documentTab != second || a.doc.Buffer.String() != "" || first.doc.Buffer.Snapshot() != firstSnapshot {
		t.Fatal("undo affected a different tab")
	}
	tester.Key(ui.Ctrl, ui.KeyY)
	if a.doc.Buffer.String() != "second document" {
		t.Fatal("tab switch lost redo history")
	}
}

func TestCloseTabCancelSaveAndLastTab(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Set("keep this")
	first := a.documentTab
	tester := ui.NewTester(a.view, 760, 600)
	tester.Key(ui.Ctrl, ui.KeyW)
	if len(a.tabs) != 1 || a.documentTab != first || a.doc.Buffer.String() != "keep this" {
		t.Fatal("canceling close discarded edits")
	}
	path := filepath.Join(t.TempDir(), "saved.txt")
	dialogs := a.dialogs.(*fakeDialogs)
	dialogs.message = func(options mygo.MessageOptions) (mygo.MessageResult, error) {
		return mygo.MessageResult{Button: 0}, nil
	}
	dialogs.save = func(mygo.SaveDialogOptions) (string, error) { return path, nil }
	tester.Key(ui.Ctrl, ui.KeyW)
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "keep this" {
		t.Fatalf("close did not save before removing tab: %q %v", got, err)
	}
	if len(a.tabs) != 1 || a.documentTab == first || a.doc.Path != "" || a.doc.Buffer.Len() != 0 {
		t.Fatal("closing the last tab did not leave a new empty document")
	}
}

func TestClosingInactiveCleanTabKeepsActiveDocument(t *testing.T) {
	a := testApp()
	first := a.documentTab
	a.newDocument()
	second := a.documentTab
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("关闭标签：" + first.name()); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || a.documentTab != second || len(a.dialogs.(*fakeDialogs).messages) != 0 {
		t.Fatal("closing an inactive clean tab changed the active document or prompted")
	}
}

func TestNativeTabsKeepKeyboardNavigationAndCloseTargets(t *testing.T) {
	a := testApp()
	a.newDocument()
	a.newDocument()
	first, second, third := a.tabs[0], a.tabs[1], a.tabs[2]
	a.activateTab(first)
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("标签：" + first.name()); err != nil {
		t.Fatal(err)
	}
	if !tester.Focused("标签：" + first.name()) {
		t.Fatal("native tab cannot take keyboard focus")
	}
	tester.Key(0, ui.KeyRight)
	if a.documentTab != second || !tester.Focused("标签："+second.name()) {
		t.Fatal("Right did not select and focus the next native tab")
	}
	tester.Key(0, ui.KeyEnd)
	if a.documentTab != third || !tester.Focused("标签："+third.name()) {
		t.Fatal("End did not select and focus the last native tab")
	}
	tester.Key(0, ui.KeyHome)
	if a.documentTab != first || !tester.Focused("标签："+first.name()) {
		t.Fatal("Home did not select and focus the first native tab")
	}
	if err := tester.Click("关闭标签：" + third.name()); err != nil {
		t.Fatal(err)
	}
	if a.documentTab != first || a.tabIndex != 0 || len(a.tabs) != 2 {
		t.Fatal("closing a different tab selected it or left the native selection out of sync")
	}
}

func TestTabsCannotChangeDuringDocumentOperation(t *testing.T) {
	a := testApp()
	first := a.documentTab
	a.newDocument()
	active := a.documentTab
	_, id := a.startJob(locales.FormattingJSON)
	tester := ui.NewTester(a.view, 760, 600)
	tester.Key(ui.Ctrl, ui.KeyTab)
	tester.Key(ui.Ctrl, ui.KeyN)
	for _, label := range []string{"标签：" + first.name(), "关闭标签：" + active.name(), "新建标签"} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.documentTab != active || len(a.tabs) != 2 {
		t.Fatal("background operation allowed its document to change or close")
	}
	a.finishJob(id, func() {})
	if err := tester.Click("标签：" + first.name()); err != nil {
		t.Fatal(err)
	}
	if a.documentTab != first {
		t.Fatal("tab remained disabled after the document operation finished")
	}
}

func TestFileArrivingDuringSavePromptCannotReplaceItsDocument(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Set("save this document")
	original := a.documentTab
	incoming := desktopFile(t, "incoming.txt", "another file")
	target := filepath.Join(t.TempDir(), "saved.txt")
	dialogs := a.dialogs.(*fakeDialogs)
	dialogs.message = func(mygo.MessageOptions) (mygo.MessageResult, error) {
		a.open(incoming)
		if a.documentTab != original || len(a.pendingOpen) != 1 {
			t.Fatal("incoming file replaced the document while confirming its save")
		}
		return mygo.MessageResult{Button: 0}, nil
	}
	dialogs.save = func(mygo.SaveDialogOptions) (string, error) { return target, nil }
	a.closeTab(original)
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "save this document" || !document.SamePath(a.doc.Path, incoming) {
		t.Fatalf("wrong document saved/loaded after prompt: saved=%q, active=%q, err=%v", got, a.doc.Path, err)
	}
}

func TestConfirmAllTabsChecksEveryUnsavedDocument(t *testing.T) {
	for _, cancelLast := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelLast), func(t *testing.T) {
			a := testApp()
			firstPath := desktopFile(t, "first.txt", "original first")
			secondPath := desktopFile(t, "second.txt", "original second")
			a.openFiles([]string{firstPath, secondPath})
			a.tabs[0].doc.Buffer.Set("saved first")
			a.tabs[1].doc.Buffer.Set("unsaved second")
			dialogs := a.dialogs.(*fakeDialogs)
			calls := 0
			dialogs.message = func(mygo.MessageOptions) (mygo.MessageResult, error) {
				calls++
				button := 0
				if calls == 2 {
					button = 1
					if cancelLast {
						button = 2
					}
				}
				return mygo.MessageResult{Button: button}, nil
			}
			continued := false
			a.confirmAllTabs(func() { continued = true })
			if calls != 2 || continued == cancelLast {
				t.Fatalf("prompts=%d, continued=%v", calls, continued)
			}
			first, _ := os.ReadFile(firstPath)
			second, _ := os.ReadFile(secondPath)
			if string(first) != "saved first" || string(second) != "original second" {
				t.Fatalf("incorrect save/discard handling: %q %q", first, second)
			}
			if cancelLast && (len(a.tabs) != 2 || a.tabs[1].doc.Buffer.String() != "unsaved second") {
				t.Fatal("canceling exit discarded a document")
			}
		})
	}
}

func TestTabOverflowRemainsReachableFromList(t *testing.T) {
	a := testApp()
	for i := 1; i < 14; i++ {
		a.newDocument()
	}
	first, last := a.tabs[0], a.documentTab
	tester := ui.NewTester(a.view, 760, 420)
	if err := tester.Click("标签：" + last.name()); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("标签列表"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem(first.name()); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if a.documentTab != first {
		t.Fatal("overflow menu did not activate the requested tab")
	}
	if err := tester.Click("标签：" + first.name()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeTabsFitAtDifferentScalesAndAfterResize(t *testing.T) {
	for _, theme := range []string{"浅色", "深色"} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%s/scale=%g", theme, scale), func(t *testing.T) {
				a := testApp()
				a.settings.Theme = theme
				for i := 0; i < 6; i++ {
					doc := document.New()
					doc.Path = fmt.Sprintf("very-long-filename-with-many-characters-%d.json", i)
					a.addDocument(doc)
				}
				tester := ui.NewTester(a.view, 1120, 420)
				tester.SetScale(scale)
				for _, width := range []int{1120, 760, 1120} {
					tester.SetSize(width, 420)
					rect, ok := tester.Find("标签：" + a.documentTab.name())
					close, closeOK := tester.Find("关闭标签：" + a.documentTab.name())
					list, listOK := tester.Find("标签列表")
					if !ok || !closeOK || !listOK || rect.W > 240.5 || rect.X < 0 ||
						close.X < rect.X || close.X+close.W > rect.X+rect.W+0.5 ||
						close.X+close.W > list.X+0.5 {
						t.Fatalf("width %d: selected tab/close clipped or oversized: tab=%+v close=%+v list=%+v", width, rect, close, list)
					}
					tester.Move(close.X+close.W/2, close.Y+close.H/2)
					tester.Frame()
					hover, _ := tester.Find("关闭标签：" + a.documentTab.name())
					if hover != close {
						t.Fatal("hovering the close action changed tab geometry")
					}
				}
			})
		}
	}
}

func TestTabsPreview(t *testing.T) {
	directory := os.Getenv("TEXTVIEWER_PREVIEW_DIR")
	if directory == "" {
		t.Skip("set TEXTVIEWER_PREVIEW_DIR for tab previews")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"浅色", "深色"} {
		a := testApp()
		a.settings.Theme = theme
		a.doc.Path = `D:\Projects\config\settings.json`
		a.doc.Buffer.Reset("{\n  \"name\": \"TextViewer\",\n  \"tabs\": true\n}\n")
		a.resetEditor()
		first := a.documentTab
		for _, name := range []string{`D:\Projects\server\main.go`, `D:\Projects\notes\阅读笔记.txt`, `D:\Projects\notes\很长的文件名称用于检查标签宽度和截断.log`, `D:\Projects\data\sample.csv`} {
			doc := document.New()
			doc.Path = name
			a.addDocument(doc)
		}
		a.activateTab(first)
		tester := ui.NewTester(a.view, 760, 420)
		for _, width := range []int{760, 1120} {
			tester.SetSize(width, 420)
			for _, scale := range []float32{1, 1.5, 2} {
				tester.SetScale(scale)
				name := fmt.Sprintf("tabs-%s-%d-%g.png", theme, width, scale)
				if width == 760 && scale == 1 {
					name = "tabs-" + theme + ".png"
				}
				file, err := os.Create(filepath.Join(directory, name))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, tester.Image())
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func BenchmarkSwitchLargeDocumentTabs(b *testing.B) {
	a := testApp()
	for range 2 {
		doc := document.New()
		doc.Buffer.Reset(strings.Repeat("0123456789abcde\n", (100<<20)/16))
		a.addDocument(doc)
	}
	first, second := a.tabs[1], a.tabs[2]
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.activateTab(first)
		a.activateTab(second)
	}
}

func TestSaveAsCopiesFileWithSelectedEncodingAndLineEndings(t *testing.T) {
	a := testApp()
	original := desktopFile(t, "original.json", "{\"old\":true}\n")
	a.open(original)
	a.doc.Buffer.Set("{\"new\":\"中文\"}\n")
	a.doc.Encoding = document.UTF16LE
	a.doc.Buffer.SetLineEnding("\r\n")
	target := filepath.Join(t.TempDir(), "copy.json")
	a.dialogs.(*fakeDialogs).save = func(options mygo.SaveDialogOptions) (string, error) {
		if options.Title != "另存为" || !document.SamePath(options.DefaultPath, original) ||
			options.Filters[0].Extensions[0] != "json" {
			t.Fatalf("save-as picker lost the original filename/type: %+v", options)
		}
		return target, nil
	}
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("另存为"); err != nil {
		t.Fatal(err)
	}
	loaded, err := document.Load(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(original)
	if string(before) != "{\"old\":true}\n" || loaded.Encoding != document.UTF16LE ||
		loaded.Buffer.LineEnding() != "\r\n" || loaded.Buffer.String() != "{\"new\":\"中文\"}\n" ||
		!document.SamePath(a.doc.Path, target) || a.doc.Dirty() || len(a.tabs) != 1 {
		t.Fatal("save as changed the source, lost output settings, or failed to retarget the tab")
	}
}

func TestSaveAsCancelAndOpenTargetDoNotChangeDocuments(t *testing.T) {
	a := testApp()
	first := desktopFile(t, "first.txt", "first")
	second := desktopFile(t, "second.txt", "second")
	a.openFiles([]string{first, second})
	a.doc.Buffer.Set("edited second")
	before := a.doc.Buffer.Snapshot()
	tester := ui.NewTester(a.view, 760, 600)
	tester.Key(ui.Ctrl|ui.Shift, ui.KeyS)
	if !document.SamePath(a.doc.Path, second) || a.doc.Buffer.Snapshot() != before || !a.doc.Dirty() {
		t.Fatal("canceling save as changed the active document")
	}
	a.dialogs.(*fakeDialogs).save = func(mygo.SaveDialogOptions) (string, error) { return first, nil }
	tester.Key(ui.Ctrl|ui.Shift, ui.KeyS)
	firstBytes, _ := os.ReadFile(first)
	if string(firstBytes) != "first" || !document.SamePath(a.doc.Path, second) ||
		a.doc.Buffer.Snapshot() != before || len(a.dialogs.(*fakeDialogs).messages) != 1 {
		t.Fatal("save as overwrote a file owned by another open tab")
	}
}

func TestSaveAsQueuesIncomingFilesWhileChoosingPath(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancel), func(t *testing.T) {
			a := testApp()
			a.doc.Buffer.Set("original unsaved text")
			original := a.documentTab
			incoming := desktopFile(t, "incoming.txt", "incoming text")
			target := filepath.Join(t.TempDir(), "saved.txt")
			a.dialogs.(*fakeDialogs).save = func(mygo.SaveDialogOptions) (string, error) {
				a.open(incoming)
				if a.documentTab != original || len(a.pendingOpen) != 1 {
					t.Fatal("incoming file changed the document while choosing its save path")
				}
				if cancel {
					return "", nil
				}
				return target, nil
			}
			a.save(true, nil)
			if original.doc.Buffer.String() != "original unsaved text" ||
				!document.SamePath(a.doc.Path, incoming) || len(a.tabs) != 2 {
				t.Fatal("save picker lost the original document or incoming path")
			}
			if cancel {
				if !original.doc.Dirty() || original.doc.Path != "" {
					t.Fatal("canceling the picker changed the original save state")
				}
			} else if got, err := os.ReadFile(target); err != nil || string(got) != "original unsaved text" || original.doc.Dirty() {
				t.Fatalf("save picker saved the wrong document: %q %v", got, err)
			}
		})
	}
}
