// Package startuptrace records only unusually slow launches. Markers run on
// the UI thread; saving a completed report does not touch application state.
package startuptrace

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

const slowStartup = 2 * time.Second

type Stage struct {
	Name string  `json:"name"`
	MS   float64 `json:"elapsedMS"`
}

type report struct {
	Version      string    `json:"version"`
	Started      time.Time `json:"started"`
	BeforeMainMS float64   `json:"beforeMainMS"`
	TotalMS      float64   `json:"totalMS"`
	Stages       []Stage   `json:"stages"`
}

type Recorder struct {
	entered    time.Time
	beforeMain time.Duration
	version    string
	stages     []Stage
	done       bool
	verbose    bool
}

func New(version string, entered time.Time) *Recorder {
	r := &Recorder{entered: entered, beforeMain: beforeMain(entered), version: version, verbose: os.Getenv("TEXTVIEWER_STARTUP_TRACE") == "1"}
	r.Mark("main")
	return r
}

// Mark is safe on a nil recorder, including tests that construct an App directly.
func (r *Recorder) Mark(name string) {
	if r == nil || r.done {
		return
	}
	r.stages = append(r.stages, Stage{name, float64(time.Since(r.entered)+r.beforeMain) / float64(time.Millisecond)})
	if r.verbose {
		log.Printf("TextViewer startup: %s %.3f ms", name, r.stages[len(r.stages)-1].MS)
	}
}

func (r *Recorder) finish(name string, now time.Time) *report {
	if r == nil || r.done {
		return nil
	}
	r.done = true
	elapsed := now.Sub(r.entered) + r.beforeMain
	if r.verbose {
		log.Printf("TextViewer startup: %s %.3f ms", name, float64(elapsed)/float64(time.Millisecond))
	}
	if elapsed < slowStartup {
		return nil
	}
	stages := append([]Stage(nil), r.stages...)
	stages = append(stages, Stage{name, float64(elapsed) / float64(time.Millisecond)})
	return &report{r.version, r.entered.Add(-r.beforeMain).UTC(), float64(r.beforeMain) / float64(time.Millisecond), float64(elapsed) / float64(time.Millisecond), stages}
}

// Complete runs at process exit if no first frame was reached (e.g. forwarding).
func (r *Recorder) Complete(name string) { save(r.finish(name, time.Now())) }

// CompleteAsync leaves disk I/O off the window thread, and records only once.
func (r *Recorder) CompleteAsync(name string) {
	if result := r.finish(name, time.Now()); result != nil {
		go save(result)
	}
}

func save(result *report) {
	if result == nil {
		return
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	_ = writeReport(filepath.Join(dir, "TextViewer"), result)
}

func writeReport(dir string, result *report) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".startup-slow-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Keep just the latest slow launch, including when a previous log exists.
	return os.Rename(f.Name(), filepath.Join(dir, "startup-slow.json"))
}
