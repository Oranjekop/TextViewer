package filetypes

import (
	"slices"
	"testing"
)

func TestCatalogIncludesCommonTextFormatsAndNoBinaryDocuments(t *testing.T) {
	extensions := Extensions()
	for _, extension := range []string{"txt", "log", "md", "json", "jsonc", "csv", "yaml", "toml", "ini", "xml", "html", "svg", "go", "js", "ts", "py", "ps1", "bat", "sh"} {
		if !slices.Contains(extensions, extension) {
			t.Errorf("missing %s", extension)
		}
	}
	for _, extension := range []string{"exe", "dll", "pdf", "docx", "zip", "png", "jpg"} {
		if slices.Contains(extensions, extension) {
			t.Errorf("registered binary format %s", extension)
		}
	}
	groups := Groups()
	groups[0].Extensions[0] = ".changed"
	if Groups()[0].Extensions[0] != ".txt" {
		t.Fatal("caller changed the shared catalog")
	}
}

func TestCatalogRejectsUnsafeOrDuplicateMappings(t *testing.T) {
	for _, data := range []string{
		`[]`,
		`[{"id":"../text","name":"Text","extensions":[".txt"]}]`,
		`[{"id":"text","name":"Text","extensions":[".txt\\bad"]}]`,
		`[{"id":"text","name":"Text","extensions":[".txt",".txt"]}]`,
		`[{"id":"text","name":"Text","extensions":[".txt"]},{"id":"text","name":"Other","extensions":[".log"]}]`,
		`[{"id":"text","name":"Text","extensions":[".txt"]},{"id":"code","name":"Code","extensions":[".txt"]}]`,
	} {
		if _, err := parseCatalog([]byte(data)); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
}
