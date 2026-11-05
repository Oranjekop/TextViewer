package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/internal/search"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type performanceReport struct {
	Language           string  `json:"language"`
	FontFamily         string  `json:"fontFamily"`
	SoftWrap           bool    `json:"softWrap"`
	FileBytes          int64   `json:"fileBytes"`
	Lines              int     `json:"lines"`
	OpenMS             float64 `json:"openMS"`
	RetainedHeapMB     float64 `json:"retainedHeapMB"`
	LanguageSetupMS    float64 `json:"languageSetupMS"`
	FirstFrameMS       float64 `json:"firstFrameMS"`
	WrapIndexMS        float64 `json:"wrapIndexMS"`
	NavigateEndMS      float64 `json:"navigateEndMS"`
	SyntaxCatchupMS    float64 `json:"syntaxCatchupMS"`
	SteadyFrameMS      float64 `json:"steadyFrameMS"`
	SteadyFrameBytes   uint64  `json:"steadyFrameBytes"`
	EditMS             float64 `json:"editMS"`
	EditAllocatedBytes uint64  `json:"editAllocatedBytes"`
	UITypeMS           float64 `json:"uiTypeMS"`
	UITypeBytes        uint64  `json:"uiTypeAllocatedBytes"`
	ScrollMS           float64 `json:"scrollMS"`
	SearchMS           float64 `json:"searchMS"`
	SearchAllocated    uint64  `json:"searchAllocatedBytes"`
	SaveMS             float64 `json:"saveMS"`
}

// PerformanceCheck uses a generated local fixture and the real native editor's
// headless renderer. It never opens or alters an existing user document.
func PerformanceCheck(args []string) error {
	flags := flag.NewFlagSet("performance-check", flag.ContinueOnError)
	size := flags.Int("size-mb", 100, "generated fixture size in MiB")
	format := flags.String("format", "txt", "fixture format: txt or json")
	fontFamily := flags.String("font", "", "editor font family; empty uses automatic selection")
	softWrap := flags.Bool("wrap", true, "enable automatic word wrap")
	output := flags.String("output", "dist/evidence", "report and preview directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *size < 1 || *size > 512 {
		return fmt.Errorf("size-mb must be between 1 and 512")
	}
	if *format != "txt" && *format != "json" {
		return fmt.Errorf("format must be txt or json")
	}
	if err := os.MkdirAll(*output, 0755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "textviewer-performance-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	path := filepath.Join(temp, "large."+*format)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	line := strings.Repeat("TextViewer native editor / 中文文本 / ", 3) + "\r\n"
	prefix, suffix := "", ""
	if *format == "json" {
		prefix, suffix = "[\r\n", "  {}\r\n]\r\n"
		line = "  {\"text\": \"" + strings.Repeat("TextViewer 中文 ", 3) + "\", \"number\": 42, \"enabled\": true},\r\n"
	}
	chunk := strings.Repeat(line, 1024)
	n, err := w.WriteString(prefix)
	if err != nil {
		f.Close()
		return err
	}
	written := int64(n)
	for written < int64(*size)<<20 {
		n, writeErr := w.WriteString(chunk)
		if writeErr != nil {
			f.Close()
			return writeErr
		}
		written += int64(n)
	}
	n, err = w.WriteString(suffix)
	written += int64(n)
	if err != nil {
		f.Close()
		return err
	}
	if err = w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	runtime.GC()
	var baseline, after runtime.MemStats
	runtime.ReadMemStats(&baseline)
	start := time.Now()
	doc, err := document.Load(context.Background(), path)
	if err != nil {
		return err
	}
	report := performanceReport{FileBytes: written, Lines: doc.Buffer.LineCount(), OpenMS: milliseconds(start)}
	runtime.GC()
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > baseline.HeapAlloc {
		report.RetainedHeapMB = float64(after.HeapAlloc-baseline.HeapAlloc) / (1 << 20)
	}
	a := New()
	a.doc = doc
	a.settings.Theme, a.fontSize, a.wrap = "浅色", "16", *softWrap
	a.settings.FontFamily = *fontFamily
	start = time.Now()
	a.resetEditor()
	report.LanguageSetupMS = milliseconds(start)
	report.Language = a.languageLabel()
	report.FontFamily, report.SoftWrap = a.editorFont(), a.wrap
	start = time.Now()
	tester := ui.NewTester(a.view, 1100, 720)
	report.FirstFrameMS = milliseconds(start)
	// Tester settles up to twenty frames per call. Let the progressive wrap
	// index finish before measuring genuinely steady frames.
	warm := time.Now()
	for tester.FramePending() && time.Since(warm) < 30*time.Second {
		tester.Frame()
	}
	report.WrapIndexMS = milliseconds(warm)
	if tester.FramePending() {
		return fmt.Errorf("initial layout did not settle within 30 seconds")
	}
	start = time.Now()
	runtime.ReadMemStats(&baseline)
	for range 5 {
		tester.Frame()
	}
	runtime.ReadMemStats(&after)
	report.SteadyFrameMS = milliseconds(start) / 5
	report.SteadyFrameBytes = (after.TotalAlloc - baseline.TotalAlloc) / 5
	start = time.Now()
	runtime.ReadMemStats(&baseline)
	at := max(0, doc.Buffer.Len()-20)
	doc.Buffer.Edit(editor.Edit{Start: at, End: at, Text: "X"})
	runtime.ReadMemStats(&after)
	report.EditMS = milliseconds(start)
	report.EditAllocatedBytes = after.TotalAlloc - baseline.TotalAlloc
	doc.Buffer.Undo()
	tester.Frame()
	start = time.Now()
	tester.Key(ui.Cmd, ui.KeyEnd)
	report.NavigateEndMS = milliseconds(start)
	warm = time.Now()
	for tester.FramePending() && time.Since(warm) < 30*time.Second {
		tester.Frame()
	}
	report.SyntaxCatchupMS = milliseconds(warm)
	if tester.FramePending() {
		return fmt.Errorf("syntax highlighting did not settle within 30 seconds")
	}
	start = time.Now()
	runtime.ReadMemStats(&baseline)
	tester.Type("X")
	runtime.ReadMemStats(&after)
	report.UITypeMS = milliseconds(start)
	report.UITypeBytes = after.TotalAlloc - baseline.TotalAlloc
	tester.Key(ui.Cmd, ui.KeyZ)
	start = time.Now()
	tester.Scroll(500, 350, 0, -500)
	report.ScrollMS = milliseconds(start)
	start = time.Now()
	runtime.ReadMemStats(&baseline)
	_, found, err := search.Next(context.Background(), doc.Buffer.Snapshot(), search.Query{Text: "this-pattern-is-not-in-the-file"}, 0, false)
	runtime.ReadMemStats(&after)
	report.SearchMS = milliseconds(start)
	report.SearchAllocated = after.TotalAlloc - baseline.TotalAlloc
	if err != nil {
		return err
	}
	if found {
		return fmt.Errorf("unexpected search result in generated fixture")
	}
	start = time.Now()
	_, err = doc.SaveSnapshot(context.Background(), filepath.Join(temp, "saved.txt"), doc.CaptureSave())
	report.SaveMS = milliseconds(start)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	reportSuffix := fmt.Sprintf("%dmb", *size)
	if *format != "txt" {
		reportSuffix = *format + "-" + reportSuffix
	}
	if err = os.WriteFile(filepath.Join(*output, "performance-"+reportSuffix+".json"), data, 0644); err != nil {
		return err
	}
	imageFile, err := os.Create(filepath.Join(*output, "preview-"+reportSuffix+".png"))
	if err != nil {
		return err
	}
	defer imageFile.Close()
	tester.Frame()
	return png.Encode(imageFile, tester.Image())
}

func milliseconds(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}
