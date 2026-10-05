package deck

import "sort"

// applyTagEdits returns the tags with the additions and removals applied,
// normalised the way the file stores them.
func ApplyTagEdits(tags, add, remove []string) []string {
	set := map[string]bool{}
	for _, t := range tags {
		set[t] = true
	}
	for _, t := range add {
		set[t] = true
	}
	for _, t := range remove {
		delete(set, t)
	}
	if len(set) == 0 {
		return nil
	}

	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
