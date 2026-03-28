package prompt

import (
	"path/filepath"
	"sort"
	"strings"
)

var focusWrapperNames = map[string]struct{}{
	"internal":       {},
	"src":            {},
	"pkg":            {},
	"lib":            {},
	"cmd":            {},
	"marketing-site": {},
}

func SummarizeFocusAreas(paths []string, limit int) []string {
	if limit <= 0 {
		return nil
	}

	counts := make(map[string]int)
	order := make(map[string]int)
	for idx, path := range paths {
		area := focusArea(path)
		if area == "" {
			continue
		}
		counts[area]++
		if _, ok := order[area]; !ok {
			order[area] = idx
		}
	}
	if len(counts) == 0 {
		return nil
	}

	areas := make([]string, 0, len(counts))
	for area := range counts {
		areas = append(areas, area)
	}
	sort.SliceStable(areas, func(i, j int) bool {
		if counts[areas[i]] == counts[areas[j]] {
			return order[areas[i]] < order[areas[j]]
		}
		return counts[areas[i]] > counts[areas[j]]
	})
	if len(areas) > limit {
		areas = areas[:limit]
	}
	return areas
}

func focusArea(path string) string {
	trimmed := strings.TrimSpace(filepath.ToSlash(path))
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "./")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}

	last := strings.TrimSpace(parts[len(parts)-1])
	if ext := filepath.Ext(last); ext != "" {
		last = strings.TrimSuffix(last, ext)
	}
	last = normalize(last)

	var labels []string
	for _, part := range parts[:len(parts)-1] {
		part = normalize(part)
		if part == "" {
			continue
		}
		if _, skip := focusWrapperNames[part]; skip {
			continue
		}
		labels = append(labels, part)
	}
	if len(labels) > 0 {
		return strings.Join(labels, " ")
	}
	return last
}
