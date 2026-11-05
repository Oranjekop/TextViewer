package desktop

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestCharacterCountUnicodeEditsAndUndo(t *testing.T) {
	a := testApp()
	tester := ui.NewTester(a.view, 760, 600)
	assertCount := func(count string) {
		t.Helper()
		tester.Frame()
		if !tester.HasText(locales.CharacterStats(count)) {
			t.Fatalf("missing character count %q", count)
		}
	}
	assertCount("0")
	tester.Type("中文😀 e\u0301")
	assertCount("6")
	tester.Key(0, ui.KeyEnter)
	assertCount("7")
	tester.Key(ui.Cmd, ui.KeyZ)
	assertCount("6")
	a.newDocument()
	assertCount("0")
	a.nextTab(-1)
	assertCount("6")
}

func TestLargeCharacterCountSnapshotAndCancellation(t *testing.T) {
	text := strings.Repeat("中😀e\u0301\n", 100000)
	buffer := editor.NewBuffer(text)
	snapshot := buffer.Snapshot()
	buffer.Set("new document")
	count, err := countCharacters(context.Background(), snapshot)
	if err != nil || count != utf8.RuneCountInString(text) {
		t.Fatalf("immutable snapshot count=%d, err=%v", count, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := countCharacters(ctx, snapshot); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestLargeCharacterCounterCachesAndDebounces(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Set(strings.Repeat("中😀\n", 30000))
	tester := ui.NewTester(a.view, 760, 600)
	wait := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !a.characters.ready && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
			tester.Frame()
		}
		if !a.characters.ready {
			t.Fatal("background count did not finish")
		}
	}
	wait()
	if a.characters.count != 90000 {
		t.Fatalf("count=%d", a.characters.count)
	}
	snapshot := a.characters.snapshot
	tester.Frame()
	if a.characters.snapshot != snapshot || a.characters.result != nil {
		t.Fatal("stable frame restarted a scan")
	}
	a.doc.Buffer.Set(strings.Repeat("ab", 40000))
	tester.Frame()
	if a.characters.ready || a.characters.result != nil {
		t.Fatal("large edit was not debounced or displayed a stale count")
	}
	tester.Advance(201 * time.Millisecond)
	wait()
	if a.characters.count != 80000 {
		t.Fatalf("updated count=%d", a.characters.count)
	}
	a.characters.release()
}

func BenchmarkCharacterCountLargeUnicode(b *testing.B) {
	buffer := editor.NewBuffer(strings.Repeat("中😀e\u0301\n", 100000))
	snapshot := buffer.Snapshot()
	b.SetBytes(int64(snapshot.Len()))
	b.ReportAllocs()
	for b.Loop() {
		_, _ = countCharacters(context.Background(), snapshot)
	}
}
