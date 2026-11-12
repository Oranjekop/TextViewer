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

//go:embed associations.json
var associationsJSON []byte

// AssociationGroups is deliberately narrower than the editor's file filters.
// Source code and runnable scripts can be opened without registering handlers.
func AssociationGroups() []Group {
	var extensions []string
	if err := json.Unmarshal(associationsJSON, &extensions); err != nil {
		panic(err)
	}
	allowed := make(map[string]bool, len(extensions))
	for _, extension := range extensions {
		allowed[extension] = true
	}
	var groups []Group
	for _, group := range Groups() {
		kept := group.Extensions[:0]
		for _, extension := range group.Extensions {
			if allowed[extension] {
				kept = append(kept, extension)
			}
		}
		if len(kept) != 0 {
			group.Extensions = kept
			groups = append(groups, group)
		}
	}
	return groups
}

// AssociationOptions offers documents and development files. The shell/batch
// script group remains excluded from selectable registrations.
func AssociationOptions() []Group {
	var groups []Group
	for _, group := range Groups() {
		if group.ID != "script" {
			groups = append(groups, group)
		}
	}
	return groups
}

// SelectedAssociationGroups validates and canonicalizes a saved selection.
// nil means the conservative default used before selections were introduced.
func SelectedAssociationGroups(extensions []string) ([]Group, error) {
	if extensions == nil {
		return AssociationGroups(), nil
	}
	selected := make(map[string]bool, len(extensions))
	for _, extension := range extensions {
		selected[extension] = true
	}
	var groups []Group
	for _, group := range AssociationOptions() {
		kept := group.Extensions[:0]
		for _, extension := range group.Extensions {
			if selected[extension] {
				kept = append(kept, extension)
				delete(selected, extension)
			}
		}
		if len(kept) > 0 {
			group.Extensions = kept
			groups = append(groups, group)
		}
	}
	if len(selected) > 0 || len(groups) == 0 {
		return nil, fmt.Errorf("invalid file association selection")
	}
	return groups, nil
}

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
