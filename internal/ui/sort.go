// Sorting: the ordering clause YouTrack itself appends to a query. `S` cycles
// it and the list is refetched, because ordering is the instance's job — the
// pages already on screen are only a window onto the result set.
package ui

import (
	"regexp"
	"strings"
)

// sortOrders is the cycle `S` walks. The empty first entry injects nothing:
// a saved search can carry its own `sort by:` clause, and the default has to
// be whatever the filter — or YouTrack, which sorts by `updated` — already
// does. Only built-in fields are offered; a custom field is named differently
// on every instance (see the dynamic-fields invariant), so `sort by: Priority
// desc` is typed into the `s` prompt instead.
var sortOrders = []string{
	"",
	"updated desc",
	"updated asc",
	"created desc",
	"created asc",
}

// sortByRe matches the ordering clause of a query. YouTrack's own helper
// appends it to the query text and it runs to the end, so replacing ours means
// cutting from there on.
//
// ponytail: a literal "sort by:" inside a quoted search term is cut too. It
// takes a query parser to tell those apart, for a query nobody writes.
var sortByRe = regexp.MustCompile(`(?i)\s*\bsort by:.*$`)

// applySort rewrites the ordering clause of query. An empty clause changes
// nothing — the filter keeps the order it was written with.
func applySort(query, clause string) string {
	if clause == "" {
		return query
	}
	return strings.TrimSpace(sortByRe.ReplaceAllString(query, "")) + " sort by: " + clause
}
