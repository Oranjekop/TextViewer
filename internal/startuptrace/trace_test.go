package startuptrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSlowStartupIncludesTimeBeforeMainAndCompletesOnce(t *testing.T) {
	entered := time.Now()
	r := &Recorder{entered: entered, beforeMain: 5 * time.Second, version: "test"}
	r.Mark("main")
	r.Mark("instance-ready")
	result := r.finish("first-frame", entered.Add(time.Second))
	if result == nil || result.TotalMS != 6000 || result.BeforeMainMS != 5000 || !result.Started.Equal(entered.Add(-5*time.Second)) {
		t.Fatalf("process load delay was lost: %+v", result)
	}
	if len(result.Stages) != 3 || result.Stages[0].MS < 5000 || result.Stages[2] != (Stage{"first-frame", 6000}) {
		t.Fatalf("incorrect phase timings: %+v", result.Stages)
	}
	r.Mark("after-startup")
	if r.finish("exit", entered.Add(time.Hour)) != nil || len(result.Stages) != 3 {
		t.Fatal("exit replaced the startup report")
	}
}

func TestFastStartupAndNilRecorderDoNotProduceReport(t *testing.T) {
	entered := time.Now()
	r := &Recorder{entered: entered}
	if r.finish("first-frame", entered.Add(time.Millisecond)) != nil {
		t.Fatal("fast startup generated a report")
	}
	var absent *Recorder
	absent.Mark("ignored")
	absent.Complete("exit")
	absent.CompleteAsync("first-frame")
}

func TestReportReplacesPreviousSlowLaunchWithoutTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	for _, version := range []string{"old", "new"} {
		if err := writeReport(dir, &report{Version: version}); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != "startup-slow.json" {
		t.Fatalf("unbounded or leftover reports: %v, %v", files, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "startup-slow.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result report
	if err := json.Unmarshal(data, &result); err != nil || result.Version != "new" {
		t.Fatalf("incorrect report: %s, %v", data, err)
	}
}

func BenchmarkStartupMarkers(b *testing.B) {
	for b.Loop() {
		r := New("test", time.Now())
		for _, name := range []string{"instance-ready", "preferences-ready", "backend-ready", "locale-ready", "window-created", "window-icon-ready", "show-returned"} {
			r.Mark(name)
		}
		r.finish("first-frame", time.Now())
	}
}
