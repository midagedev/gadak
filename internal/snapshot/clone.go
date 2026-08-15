package snapshot

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func planIssues(src []issueRow, scale int) []plannedIssue {
	n := len(src)
	target := n
	if scale > n {
		target = scale
	}
	out := make([]plannedIssue, 0, target)
	for i := 0; i < n; i++ {
		out = append(out, plannedIssue{src: src[i], cloneSeq: 0})
	}
	seq := 1
	for len(out) < target {
		base := src[(len(out)-n)%n]
		out = append(out, plannedIssue{src: base, cloneSeq: seq})
		seq++
	}
	return out
}

func applySpread(planned []plannedIssue, window time.Duration, now time.Time) {
	if window <= 0 || len(planned) == 0 {
		return
	}
	start := now.Add(-window)

	// Collect original created times for the first occurrence of each source
	// (clones share their source's relative placement index by sequence order).
	// We place ALL planned issues' created' evenly/linearly by their position
	// in the planned list (which preserves original order, then clones).
	n := len(planned)
	createds := make([]time.Time, n)
	if n == 1 {
		createds[0] = start
	} else {
		// Prefer linear map of original created range onto [start, now] for
		// originals; clones continue the sequence evenly after.
		// Simpler contract: even spacing preserves order and spans the window.
		for i := 0; i < n; i++ {
			frac := float64(i) / float64(n-1)
			createds[i] = start.Add(time.Duration(frac * float64(window)))
		}
	}

	for i := range planned {
		p := &planned[i]
		srcCreated, okC := parseTime(p.src.createdAt)
		srcUpdated, okU := parseTime(p.src.updatedAt)
		if !okC {
			srcCreated = start
		}
		if !okU || srcUpdated.Before(srcCreated) {
			srcUpdated = srcCreated
		}
		dstCreated := createds[i]
		dur := srcUpdated.Sub(srcCreated)
		dstUpdated := dstCreated.Add(dur)
		if dstUpdated.Before(dstCreated) {
			dstUpdated = dstCreated
		}

		p.useMap = true
		p.srcLo, p.srcHi = srcCreated, srcUpdated
		p.dstLo, p.dstHi = dstCreated, dstUpdated
		p.zeroSpan = !srcUpdated.After(srcCreated)

		p.createdAt = formatTime(dstCreated)
		p.updatedAt = formatTime(dstUpdated)
		p.itemCreatedAt = p.createdAt
		p.itemUpdatedAt = p.updatedAt
		p.itemSyncedAt = formatTime(now)

		// Remap issue-level optional timestamps.
		if v, ok := p.src.issueCols["status_changed_at"].(string); ok && v != "" {
			p.statusChangedAt = mapOrEven(v, p, 0, 1)
		}
		if v, ok := p.src.issueCols["resolved_at"].(string); ok && v != "" {
			p.resolvedAt = mapOrEven(v, p, 0, 1)
		}
		if v, ok := p.src.issueCols["reopened_at"].(string); ok && v != "" {
			p.reopenedAt = mapOrEven(v, p, 0, 1)
		}
		if v, ok := p.src.issueCols["assignee_changed_at"].(string); ok && v != "" {
			p.assigneeChangedAt = mapOrEven(v, p, 0, 1)
		}
	}
}

func mapOrEven(s string, p *plannedIssue, idx, total int) string {
	if !p.useMap {
		return s
	}
	if p.zeroSpan {
		if total <= 1 {
			return formatTime(p.dstLo)
		}
		frac := float64(idx) / float64(total-1)
		return formatTime(p.dstLo.Add(time.Duration(frac * float64(p.dstHi.Sub(p.dstLo)))))
	}
	return mapTimeString(s, p.srcLo, p.srcHi, p.dstLo, p.dstHi)
}

func maxKeyNums(issues []issueRow) map[string]int {
	m := map[string]int{}
	for _, is := range issues {
		proj, num, ok := splitKey(is.key)
		if !ok {
			proj = is.projectKey
			if proj == "" {
				proj = "SNAP"
			}
		}
		if num > m[proj] {
			m[proj] = num
		}
		// Also track by project_key column.
		if is.projectKey != "" && num > m[is.projectKey] {
			m[is.projectKey] = num
		}
	}
	return m
}

func nextKey(projectKey, srcKey string, next map[string]int) string {
	proj, _, ok := splitKey(srcKey)
	if !ok || proj == "" {
		proj = projectKey
	}
	if proj == "" {
		proj = "SNAP"
	}
	next[proj]++
	return fmt.Sprintf("%s-%d", proj, next[proj])
}

func splitKey(key string) (string, int, bool) {
	i := strings.LastIndex(key, "-")
	if i <= 0 || i == len(key)-1 {
		return "", 0, false
	}
	n, err := strconv.Atoi(key[i+1:])
	if err != nil {
		return "", 0, false
	}
	return key[:i], n, true
}
