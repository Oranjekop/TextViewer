package locales

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/i18n"
)

func TestCatalogsTranslateEveryMessage(t *testing.T) {
	keys := func(name string) []string {
		t.Helper()
		data, err := files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			t.Fatal(err)
		}
		var result []string
		for key, value := range catalog {
			if value == "" {
				t.Fatalf("empty message %s in %s", key, name)
			}
			result = append(result, key)
		}
		slices.Sort(result)
		return result
	}
	if !reflect.DeepEqual(keys("en.json"), keys("zh-Hans.json")) {
		t.Fatal("Chinese and English catalogs have different keys")
	}
	if got := In("en").DocumentStats("0 B", 1); got != "0 B  ·  1 line" {
		t.Fatalf("incorrect singular line count: %q", got)
	}
	if got := In("en").ReplacedCount(1); got != "Replaced 1 match" {
		t.Fatalf("incorrect singular replacement count: %q", got)
	}
	for _, tag := range []string{"en", "zh-Hans"} {
		msgs := In(tag)
		if text := msgs.ReplacementTip(); !strings.Contains(text, "${") || !strings.Contains(text, "$$") {
			t.Fatalf("ICU interpreted the replacement example: %q", text)
		}
		if text := msgs.FontPreview(); !strings.Contains(text, `{ "text": "TextViewer" }`) {
			t.Fatalf("ICU interpreted the font sample: %q", text)
		}
		if text := msgs.HexEscapeDigits("u", "4"); !strings.Contains(text, `\u`) || !strings.Contains(text, "4") {
			t.Fatalf("hex escape placeholders lost: %q", text)
		}
	}
	In("en").Settings()
	if got := testing.AllocsPerRun(100, func() { _ = In("en").Settings() }); got != 0 {
		t.Fatalf("cached static message allocated %g times", got)
	}
}

func TestPendingMessagesAndWrappedErrorsFollowLanguage(t *testing.T) {
	var english atomic.Bool
	i18n.SetLocaleFunc(func() string {
		if english.Load() {
			return "en"
		}
		return "zh-Hans"
	})
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	cause := errors.New("disk full")
	err := WrapError(cause, SaveFailed)
	message := Message(SettingsSaved)
	if message.String() != "设置已保存" || err.Error() != "保存失败：disk full" {
		t.Fatalf("Chinese messages failed: %s, %v", message, err)
	}
	english.Store(true)
	if message.String() != "Settings saved" || err.Error() != "Save failed: disk full" || !errors.Is(err, cause) {
		t.Fatalf("existing messages did not follow locale or lost their cause: %s, %v", message, err)
	}
}
