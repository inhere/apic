package spec

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/inhere/apic/internal/model"
)

// AssignNames assigns unique command names, returning a warning for each
// disambiguation. opIDs corresponds positionally to ops.
func AssignNames(ops []model.Operation, opIDs []string) (warnings []string) {
	segments := make([][]string, len(ops))
	var pathIndexes []int
	for i := range ops {
		if opIDs[i] != "" {
			ops[i].Name = kebab(opIDs[i])
			continue
		}
		pathIndexes = append(pathIndexes, i)
		for _, part := range strings.Split(ops[i].Path, "/") {
			if part != "" && !strings.ContainsAny(part, "{}") {
				segments[i] = append(segments[i], part)
			}
		}
	}
	prefix := 0
	if len(pathIndexes) >= 2 {
		for {
			first := segments[pathIndexes[0]]
			if prefix >= len(first)-1 {
				break
			}
			match := true
			for _, i := range pathIndexes[1:] {
				if prefix >= len(segments[i])-1 || segments[i][prefix] != first[prefix] {
					match = false
					break
				}
			}
			if !match {
				break
			}
			prefix++
		}
	}
	for _, i := range pathIndexes {
		ops[i].Name = pathName(strings.Join(segments[i][prefix:], "-"))
		if ops[i].Name == "" {
			ops[i].Name = "root"
		}
	}
	warnChange := func(i int, name string) {
		if name != ops[i].Name {
			warnings = append(warnings, fmt.Sprintf("operation %s %s renamed %q to %q", ops[i].Method, ops[i].Path, ops[i].Name, name))
			ops[i].Name = name
		}
	}
	for _, group := range nameGroups(ops) {
		methods := make(map[string]bool)
		for _, i := range group {
			methods[strings.ToUpper(ops[i].Method)] = true
		}
		if len(methods) == len(group) {
			for _, i := range group {
				warnChange(i, kebab(ops[i].Method)+"-"+ops[i].Name)
			}
		}
	}
	for _, group := range nameGroups(ops) {
		params := make(map[string]bool)
		for _, i := range group {
			params[pathParamSuffix(ops[i].Path)] = true
		}
		if len(params) > 1 {
			for _, i := range group {
				if suffix := pathParamSuffix(ops[i].Path); suffix != "" {
					warnChange(i, ops[i].Name+"-by-"+suffix)
				}
			}
		}
	}
	// Reserve every existing name so a generated suffix cannot steal a name
	// that was already unique before this final disambiguation pass.
	reserved := make(map[string]bool, len(ops))
	for _, op := range ops {
		reserved[op.Name] = true
	}
	seen := make(map[string]bool, len(ops))
	for i := range ops {
		base := ops[i].Name
		if !seen[base] {
			seen[base] = true
			continue
		}
		for n := 2; ; n++ {
			candidate := base + "-" + strconv.Itoa(n)
			if !reserved[candidate] {
				reserved[candidate] = true
				seen[candidate] = true
				warnChange(i, candidate)
				break
			}
		}
	}
	return warnings
}

// nameGroups returns duplicate groups in first-occurrence order.
func nameGroups(ops []model.Operation) [][]int {
	indexes := make(map[string]int)
	var groups [][]int
	for i, op := range ops {
		group, ok := indexes[op.Name]
		if !ok {
			group = len(groups)
			indexes[op.Name] = group
			groups = append(groups, nil)
		}
		groups[group] = append(groups[group], i)
	}
	var duplicates [][]int
	for _, group := range groups {
		if len(group) > 1 {
			duplicates = append(duplicates, group)
		}
	}
	return duplicates
}

func pathParamSuffix(path string) string {
	var names []string
	for _, match := range pathParameter.FindAllStringSubmatch(path, -1) {
		if name := pathName(match[1]); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, "-and-")
}

// pathName normalizes static path segments without splitting camel-case words.
func pathName(s string) string {
	var b strings.Builder
	separator := false
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			separator = b.Len() > 0
			continue
		}
		if separator {
			b.WriteByte('-')
			separator = false
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// kebab splits camel-case and acronym boundaries, as well as punctuation.
func kebab(s string) string {
	runes := []rune(s)
	var b strings.Builder
	separator := false
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			separator = b.Len() > 0
			continue
		}
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
				separator = b.Len() > 0
			}
		}
		if separator {
			b.WriteByte('-')
			separator = false
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
