package service

import "strings"

// normalizeMatch lower-cases and strips spaces/punctuation so "Human Resources", "human-resources"
// and "HUMAN RESOURCES" all compare equal — the minimal normalization ROPA-18's "fuzzy" master-data
// matching needs against free-text Excel cells.
func normalizeMatch(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r == ' ' || r == '-' || r == '_' || r == '.' || r == '/' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// levenshtein is the classic edit-distance function, used only as a last resort after exact and
// substring matches fail — ROPA-18's Excel cells are free text (e.g. a department name typed
// slightly differently each year), not controlled vocabulary.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// fuzzyBestMatch returns the index into candidates whose normalized text best matches query:
// an exact normalized match wins outright; otherwise the candidate with the lowest edit distance
// wins, but only when that distance is within 30% of the longer string's length (a candidate with
// nothing in common with query is never silently picked). ok is false when nothing qualifies, or
// when the query itself is empty.
func fuzzyBestMatch(query string, candidates []string) (idx int, ok bool) {
	nq := normalizeMatch(query)
	if nq == "" {
		return 0, false
	}
	for i, c := range candidates {
		if normalizeMatch(c) == nq {
			return i, true
		}
	}
	best, bestDist := -1, 1<<31-1
	for i, c := range candidates {
		nc := normalizeMatch(c)
		if nc == "" {
			continue
		}
		d := levenshtein(nq, nc)
		if d < bestDist {
			best, bestDist = i, d
		}
	}
	if best < 0 {
		return 0, false
	}
	longest := len(nq)
	if l := len([]rune(normalizeMatch(candidates[best]))); l > longest {
		longest = l
	}
	if longest == 0 || float64(bestDist)/float64(longest) > 0.3 {
		return 0, false
	}
	return best, true
}
