package changelog

import "slices"

func filterByLabels(labels []string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, label := range labels {
		if slices.Contains(filters, label) {
			return true
		}
	}
	return false
}
