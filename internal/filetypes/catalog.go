package filetypes

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type Group struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`
}

//go:embed catalog.json
var catalogJSON []byte

var catalog = func() []Group {
	groups, err := parseCatalog(catalogJSON)
	if err != nil {
		panic(err)
	}
	return groups
}()

func parseCatalog(data []byte) ([]Group, error) {
	var groups []Group
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, err
	}
	idPattern, extPattern := regexp.MustCompile(`^[a-z]+$`), regexp.MustCompile(`^\.[a-z0-9]+$`)
	ids, extensions := map[string]bool{}, map[string]bool{}
	for _, group := range groups {
		if !idPattern.MatchString(group.ID) || ids[group.ID] || group.Name == "" || len(group.Extensions) == 0 {
			return nil, fmt.Errorf("invalid file type group %q", group.ID)
		}
		ids[group.ID] = true
		for _, extension := range group.Extensions {
			if !extPattern.MatchString(extension) || extensions[extension] {
				return nil, fmt.Errorf("invalid or repeated file extension %q", extension)
			}
			extensions[extension] = true
		}
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("empty file type catalog")
	}
	return groups, nil
}

func Groups() []Group {
	groups := make([]Group, len(catalog))
	for i, group := range catalog {
		groups[i] = group
		groups[i].Extensions = append([]string(nil), group.Extensions...)
	}
	return groups
}

func Extensions() []string {
	var extensions []string
	for _, group := range catalog {
		for _, extension := range group.Extensions {
			extensions = append(extensions, strings.TrimPrefix(extension, "."))
		}
	}
	return extensions
}

func ProgID(extension string) string {
	return "Oranjekop.TextViewer." + strings.TrimPrefix(extension, ".")
}

func (g Group) IconName() string { return "FileIcon-" + g.ID + ".ico" }
