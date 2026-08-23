package jql

import (
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/calendar"
	"github.com/midagedev/gadak/internal/jira"
)

// Parse turns a JQL string or a Jira navigator URL into a Filter. Unsupported
// clauses are listed; they are not applied.
func Parse(input string, opts Opts) Result {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	ex := Extract(input)
	res := Result{Input: strings.TrimSpace(input), Filters: EmptyFilter()}
	if strings.TrimSpace(input) == "" {
		res.Error = ErrEmpty
		res.Message = "empty query"
		return res
	}
	if ex.FilterID != "" && ex.JQL == "" {
		res.Error = ErrFilterID
		res.Message = "saved filter ids are not in the mirror — open the filter in Jira and copy the JQL"
		return res
	}
	jql := ex.JQL
	if jql == "" && ex.IsURL {
		res.Error = ErrNotJQL
		res.Message = "URL has no jql= parameter"
		return res
	}
	if jql == "" {
		jql = strings.TrimSpace(input)
	}
	res.JQL = jql

	n, orders, err := parse(jql)
	if err != nil {
		res.Error = ErrParse
		res.Message = err.Error()
		return res
	}
	if n == nil {
		res.Error = ErrParse
		res.Message = "empty query"
		return res
	}

	c := &compiler{opts: opts, f: EmptyFilter(), seenMultiAND: map[string]bool{}}
	c.compileTop(n)
	c.applyOrder(orders)
	res.Filters = c.f
	res.Display = c.d
	res.Applied = c.applied
	res.Unsupported = c.unsupported
	if c.keyCount > MaxKeys {
		res.Error = ErrTooManyKeys
		res.Message = KeyLimitMessage(c.keyCount)
		return res
	}
	res.JQL, res.Omitted = Emit(c.f, c.d, EmitOpts{Email: opts.Email, AccountID: opts.AccountID})
	return res
}

type compiler struct {
	opts         Opts
	f            Filter
	d            Display
	applied      []string
	unsupported  []string
	seenMultiAND map[string]bool
	keyCount     int // set when Keys exceeds MaxKeys
}

func (c *compiler) skip(clause string) {
	if clause == "" {
		return
	}
	for _, u := range c.unsupported {
		if u == clause {
			return
		}
	}
	c.unsupported = append(c.unsupported, clause)
}

func (c *compiler) mark(name string) {
	for _, a := range c.applied {
		if a == name {
			return
		}
	}
	c.applied = append(c.applied, name)
}

func (c *compiler) compileTop(n *node) {
	for _, term := range flattenAnd(n) {
		c.compileTerm(term)
	}
}

func flattenAnd(n *node) []*node {
	if n == nil {
		return nil
	}
	if n.kind == nAnd {
		return append(flattenAnd(n.left), flattenAnd(n.right)...)
	}
	return []*node{n}
}

func flattenOr(n *node) []*node {
	if n == nil {
		return nil
	}
	if n.kind == nOr {
		return append(flattenOr(n.left), flattenOr(n.right)...)
	}
	return []*node{n}
}

func (c *compiler) compileTerm(n *node) {
	if n == nil {
		return
	}
	switch n.kind {
	case nNot:
		c.skip("NOT " + termRender(n.left) + " (negation is not in the subset)")
	case nOr:
		c.compileOr(n)
	case nClause:
		c.compileClause(n.clause)
	case nAnd:
		// Should have been flattened.
		c.compileTop(n)
	}
}

func termRender(n *node) string {
	if n == nil {
		return ""
	}
	switch n.kind {
	case nClause:
		return n.clause.render()
	case nNot:
		return "NOT " + termRender(n.left)
	case nAnd:
		return "(" + termRender(n.left) + " AND " + termRender(n.right) + ")"
	case nOr:
		return "(" + termRender(n.left) + " OR " + termRender(n.right) + ")"
	default:
		return ""
	}
}

func (c *compiler) compileOr(n *node) {
	parts := flattenOr(n)
	var field string
	var clauses []*clause
	for _, p := range parts {
		if p.kind != nClause {
			c.skip(termRender(n) + " (OR across different fields is not a gadak filter)")
			return
		}
		f := canonicalField(p.clause.field)
		if field == "" {
			field = f
		} else if f != field {
			c.skip(termRender(n) + " (OR across different fields is not a gadak filter)")
			return
		}
		clauses = append(clauses, p.clause)
	}
	// Same field: collapse to IN, unless an op we cannot fold.
	var values []value
	op := opIn
	for _, cl := range clauses {
		switch cl.op {
		case opEq, opIn:
			values = append(values, cl.values...)
		case opTilde:
			// text ~ a OR text ~ b → join
			if field == "text" || field == "summary" || field == "description" || field == "comment" {
				values = append(values, cl.values...)
				op = opTilde
				continue
			}
			c.skip(cl.render())
			return
		default:
			c.skip(termRender(n))
			return
		}
	}
	c.compileClause(&clause{field: field, op: op, values: values, raw: termRender(n)})
}

func canonicalField(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "")
	switch s {
	case "project", "projectkey":
		return "project"
	case "status":
		return "status"
	case "statuscategory", "statuscategoryid":
		return "statuscategory"
	case "assignee":
		return "assignee"
	case "reporter":
		return "reporter"
	case "label", "labels":
		return "labels"
	case "priority":
		return "priority"
	case "issuetype", "type":
		return "type"
	case "component", "components":
		return "component"
	case "fixversion", "fixversions":
		return "fixversion"
	case "created", "createddate":
		return "created"
	case "updated", "updateddate":
		return "updated"
	case "due", "duedate":
		return "duedate"
	case "resolved", "resolutiondate", "resolveddate":
		return "resolved"
	case "text", "summary", "description", "comment":
		return "text"
	case "key", "issuekey", "issue":
		return "key"
	case "parent":
		return "parent"
	case "sprint":
		return "sprint"
	case "resolution":
		return "resolution"
	default:
		return s
	}
}

func (c *compiler) compileClause(cl *clause) {
	if cl == nil {
		return
	}
	field := canonicalField(cl.field)
	switch field {
	case "project":
		if c.notInto("project", cl, &c.f.JiraProjectNot) {
			return
		}
		c.eqOrIn("project", cl, func(vs []string) {
			c.f.JiraProject = mergeUnique(c.f.JiraProject, vs)
			c.mark("project")
		})
	case "status":
		if c.notInto("status", cl, &c.f.StatusNot) {
			return
		}
		c.eqOrIn("status", cl, func(vs []string) {
			c.f.Status = mergeUnique(c.f.Status, vs)
			c.mark("status")
		})
	case "statuscategory":
		c.compileStatusCategory(cl)
	case "assignee":
		c.compileAssignee(cl)
	case "reporter":
		c.compileReporter(cl)
	case "labels":
		if c.notInto("labels", cl, &c.f.LabelsNot) {
			return
		}
		c.multiValued("labels", cl, &c.f.Labels)
	case "priority":
		if c.notInto("priority", cl, &c.f.PriorityNot) {
			return
		}
		c.eqOrIn("priority", cl, func(vs []string) {
			c.f.Priority = mergeUnique(c.f.Priority, vs)
			c.mark("priority")
		})
	case "type":
		if c.notInto("type", cl, &c.f.IssueTypeNot) {
			return
		}
		c.eqOrIn("type", cl, func(vs []string) {
			c.f.IssueType = mergeUnique(c.f.IssueType, vs)
			c.mark("type")
		})
	case "component":
		if c.notInto("component", cl, &c.f.ComponentsNot) {
			return
		}
		c.multiValued("component", cl, &c.f.Components)
	case "fixversion":
		if c.notInto("fixVersion", cl, &c.f.FixVersionsNot) {
			return
		}
		c.multiValued("fixVersion", cl, &c.f.FixVersions)
	case "created":
		c.compileDate("created", cl, &c.f.CreatedFrom, &c.f.CreatedTo)
	case "updated":
		c.compileDate("updated", cl, &c.f.UpdatedFrom, &c.f.UpdatedTo)
	case "duedate":
		c.compileDate("duedate", cl, &c.f.DueFrom, &c.f.DueTo)
	case "resolved":
		c.compileDate("resolved", cl, &c.f.ResolvedFrom, &c.f.ResolvedTo)
	case "text":
		c.compileText(cl)
	case "key":
		c.compileKey(cl)
	case "parent":
		c.compileParent(cl)
	case "sprint":
		c.compileSprint(cl)
	case "resolution":
		c.compileResolution(cl)
	default:
		c.skip(cl.render() + " (not in the subset)")
	}
}

func (c *compiler) eqOrIn(name string, cl *clause, apply func([]string)) {
	switch cl.op {
	case opEq, opIn:
		vs := c.plainValues(cl)
		if vs == nil {
			return
		}
		apply(vs)
	default:
		c.skip(cl.render() + " (only = and IN)")
	}
}

func (c *compiler) multiValued(name string, cl *clause, dest *[]string) {
	switch cl.op {
	case opEq, opIn:
		if cl.op == opEq && c.seenMultiAND[name] {
			// A second AND-ed equality on a multi-valued field means "has
			// both", which ViewFilters cannot express. Drop the field rather
			// than silently OR.
			c.skip(name + " = … AND " + name + " = … (has-all is not a gadak filter)")
			*dest = nil
			c.unmark(name)
			return
		}
		if cl.op == opEq {
			c.seenMultiAND[name] = true
		}
		vs := c.plainValues(cl)
		if vs == nil {
			return
		}
		*dest = mergeUnique(*dest, vs)
		c.mark(name)
	default:
		c.skip(cl.render() + " (only = and IN)")
	}
}

// notInto consumes an exclusion clause (NOT IN or !=) into dest and reports
// whether it did. False means the clause is not an exclusion — the caller's
// include path decides. GDK-771: every multi axis has a negation twin.
func (c *compiler) notInto(name string, cl *clause, dest *[]string) bool {
	if cl.op != opNotIn && cl.op != opNeq {
		return false
	}
	vs := c.plainValues(cl)
	if vs == nil {
		return true // refused inside plainValues (already skipped)
	}
	*dest = mergeUnique(*dest, vs)
	c.mark(name)
	return true
}

func (c *compiler) unmark(name string) {
	out := c.applied[:0]
	for _, a := range c.applied {
		if a != name {
			out = append(out, a)
		}
	}
	c.applied = out
}

func (c *compiler) plainValues(cl *clause) []string {
	var out []string
	for _, v := range cl.values {
		if v.kind == valFunc {
			if strings.EqualFold(v.funcName, "currentUser") {
				out = append(out, "currentUser()")
				continue
			}
			c.skip(cl.render() + " (function " + v.funcName + " is not in the subset)")
			return nil
		}
		s := strings.TrimSpace(v.raw)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (c *compiler) compileStatusCategory(cl *clause) {
	neg := cl.op == opNotIn || cl.op == opNeq
	if !neg && cl.op != opEq && cl.op != opIn {
		c.skip(cl.render() + " (only =, !=, IN, NOT IN)")
		return
	}
	var cats []string
	for _, v := range cl.values {
		mapped, ok := mapStatusCategory(v.raw)
		if !ok {
			c.skip(cl.render() + " (unknown statusCategory)")
			return
		}
		cats = append(cats, mapped)
	}
	if neg {
		c.f.StatusCategoryNot = mergeUnique(c.f.StatusCategoryNot, cats)
	} else {
		c.f.StatusCategory = mergeUnique(c.f.StatusCategory, cats)
	}
	c.mark("statusCategory")
}

func mapStatusCategory(raw string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "")
	switch s {
	case "todo", "2":
		return "new", true
	case "4":
		return "inprogress", true
	case "3":
		return "done", true
	}
	return jira.KnownCategory(s)
}

func (c *compiler) compileAssignee(cl *clause) {
	switch cl.op {
	case opIsEmpty:
		c.f.Unassigned = true
		c.mark("assignee")
	case opIsNotEmpty:
		c.skip(cl.render() + " (assignee is not EMPTY is not a gadak filter)")
	case opEq, opIn:
		vs := c.plainValues(cl)
		if vs == nil {
			return
		}
		c.f.AssigneeEmail = mergeUnique(c.f.AssigneeEmail, vs)
		c.mark("assignee")
	case opNotIn, opNeq:
		vs := c.plainValues(cl)
		if vs == nil {
			return
		}
		c.f.AssigneeEmailNot = mergeUnique(c.f.AssigneeEmailNot, vs)
		c.mark("assignee")
	default:
		c.skip(cl.render() + " (only =, !=, IN, NOT IN, IS EMPTY)")
	}
}

func (c *compiler) compileReporter(cl *clause) {
	switch cl.op {
	case opEq, opIn:
		vs := c.plainValues(cl)
		if vs == nil {
			return
		}
		c.f.ReporterEmail = mergeUnique(c.f.ReporterEmail, vs)
		c.mark("reporter")
	case opNotIn, opNeq:
		vs := c.plainValues(cl)
		if vs == nil {
			return
		}
		c.f.ReporterEmailNot = mergeUnique(c.f.ReporterEmailNot, vs)
		c.mark("reporter")
	default:
		c.skip(cl.render() + " (only =, !=, IN, NOT IN)")
	}
}

func (c *compiler) compileText(cl *clause) {
	if cl.op != opTilde && cl.op != opEq {
		c.skip(cl.render() + " (only ~)")
		return
	}
	var parts []string
	if c.f.Q != "" {
		parts = append(parts, c.f.Q)
	}
	for _, v := range cl.values {
		if s := strings.TrimSpace(v.raw); s != "" {
			parts = append(parts, s)
		}
	}
	c.f.Q = strings.Join(parts, " ")
	c.mark("text")
}

func (c *compiler) compileKey(cl *clause) {
	if cl.op != opEq && cl.op != opIn {
		c.skip(cl.render() + " (only = and IN)")
		return
	}
	vs := c.plainValues(cl)
	if vs == nil {
		return
	}
	c.f.Keys = mergeUniqueUpper(c.f.Keys, vs)
	c.keyCount = len(c.f.Keys)
	c.mark("key")
}

func (c *compiler) compileParent(cl *clause) {
	if cl.op != opEq && cl.op != opIn {
		c.skip(cl.render() + " (only = and IN)")
		return
	}
	vs := c.plainValues(cl)
	if vs == nil {
		return
	}
	c.f.Parent = mergeUniqueUpper(c.f.Parent, vs)
	c.mark("parent")
}

func (c *compiler) compileSprint(cl *clause) {
	if cl.op != opEq && cl.op != opIn {
		c.skip(cl.render() + " (not in the subset)")
		return
	}
	var ids []string
	open := false
	for _, v := range cl.values {
		if v.kind == valFunc {
			if strings.EqualFold(v.funcName, "openSprints") && len(v.args) == 0 && cl.op == opIn {
				open = true
				continue
			}
			c.skip(cl.render() + " (not in the subset)")
			return
		}
		s := strings.TrimSpace(v.raw)
		if s == "" {
			continue
		}
		if _, err := strconv.ParseInt(s, 10, 64); err != nil {
			c.skip(cl.render() + " (not in the subset)")
			return
		}
		ids = append(ids, s)
	}
	if open && len(ids) > 0 {
		c.skip(cl.render() + " (not in the subset)")
		return
	}
	if open {
		c.f.SprintState = mergeUnique(c.f.SprintState, []string{"active"})
		c.mark("sprint")
		return
	}
	if len(ids) == 0 {
		c.skip(cl.render() + " (not in the subset)")
		return
	}
	c.f.SprintIDs = mergeUnique(c.f.SprintIDs, ids)
	c.mark("sprint")
}

func (c *compiler) compileResolution(cl *clause) {
	switch cl.op {
	case opIsEmpty:
		// Unresolved ≈ not done. Inclusion filter: both open buckets.
		c.f.StatusCategory = mergeUnique(c.f.StatusCategory, []string{"new", "inprogress"})
		c.mark("resolution")
	case opIsNotEmpty:
		c.f.StatusCategory = mergeUnique(c.f.StatusCategory, []string{"done"})
		c.mark("resolution")
	default:
		c.skip(cl.render() + " (only IS EMPTY / IS NOT EMPTY)")
	}
}

func (c *compiler) compileDate(name string, cl *clause, from, to **string) {
	switch cl.op {
	case opGte, opGt, opLte, opLt, opEq:
	default:
		c.skip(cl.render() + " (only date comparisons)")
		return
	}
	if len(cl.values) == 0 {
		c.skip(cl.render())
		return
	}
	t, ok := evalTime(cl.values[0], c.opts.Now)
	if !ok {
		c.skip(cl.render() + " (date not in the subset)")
		return
	}
	day := calendar.FormatDay(t, calendar.In(t.Location()))
	switch cl.op {
	case opGte:
		*from = strptr(laterDate(*from, day))
	case opGt:
		*from = strptr(laterDate(*from, addDays(day, 1)))
	case opLte:
		*to = strptr(earlierDate(*to, day))
	case opLt:
		*to = strptr(earlierDate(*to, addDays(day, -1)))
	case opEq:
		*from = strptr(laterDate(*from, day))
		*to = strptr(earlierDate(*to, day))
	}
	c.mark(name)
}

func (c *compiler) applyOrder(orders []order) {
	if len(orders) == 0 {
		return
	}
	for i, o := range orders {
		key := strings.ToLower(o.field)
		var sort string
		switch key {
		case "updated", "updateddate":
			sort = "updated"
		case "created", "createddate":
			sort = "created"
		case "due", "duedate":
			sort = "due"
		case "priority":
			sort = "priority"
		default:
			c.skip("ORDER BY " + o.field + " (not a gadak sort)")
			continue
		}
		if i > 0 && c.d.Sort != "" {
			c.skip("ORDER BY " + o.field + " (only the first sort key is applied)")
			continue
		}
		c.d.Sort = sort
		if o.dir == "asc" || o.dir == "desc" {
			c.d.Dir = o.dir
		} else {
			c.d.Dir = "desc"
		}
		c.mark("ORDER BY")
	}
}

func evalTime(v value, now time.Time) (time.Time, bool) {
	switch v.kind {
	case valDuration:
		return applyDuration(now, v.raw)
	case valString, valIdent, valNumber:
		return parseAbsolute(v.raw, now)
	case valFunc:
		name := strings.ToLower(v.funcName)
		base := now
		if len(v.args) == 1 {
			var ok bool
			base, ok = evalTime(v.args[0], now)
			if !ok {
				return time.Time{}, false
			}
		} else if len(v.args) > 1 {
			return time.Time{}, false
		}
		y, m, d := base.Date()
		loc := base.Location()
		switch name {
		case "now":
			if len(v.args) > 0 {
				return time.Time{}, false
			}
			return now, true
		case "startofday":
			return time.Date(y, m, d, 0, 0, 0, 0, loc), true
		case "endofday":
			return time.Date(y, m, d, 23, 59, 59, 0, loc), true
		default:
			return time.Time{}, false
		}
	default:
		return time.Time{}, false
	}
}

func applyDuration(now time.Time, raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	sign := 1
	s := raw
	if s[0] == '-' {
		sign = -1
		s = s[1:]
	}
	if len(s) < 2 {
		return time.Time{}, false
	}
	unit := s[len(s)-1]
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return time.Time{}, false
	}
	n *= sign
	switch unit {
	case 'd', 'D':
		return now.AddDate(0, 0, n), true
	case 'w', 'W':
		return now.AddDate(0, 0, n*7), true
	case 'm', 'M':
		return now.AddDate(0, n, 0), true
	case 'y', 'Y':
		return now.AddDate(n, 0, 0), true
	case 'h', 'H':
		return now.Add(time.Duration(n) * time.Hour), true
	case 's', 'S':
		return now.Add(time.Duration(n) * time.Second), true
	default:
		return time.Time{}, false
	}
}

func parseAbsolute(raw string, now time.Time) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		"2006-01-02",
		"2006/01/02",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, now.Location()); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func addDays(day string, n int) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func laterDate(cur *string, next string) string {
	if cur == nil || *cur < next {
		return next
	}
	return *cur
}

func earlierDate(cur *string, next string) string {
	if cur == nil || *cur > next {
		return next
	}
	return *cur
}

func strptr(s string) *string { return &s }

func mergeUnique(dst, src []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, d := range dst {
		seen[strings.ToLower(d)] = true
	}
	for _, s := range src {
		k := strings.ToLower(s)
		if seen[k] {
			continue
		}
		seen[k] = true
		dst = append(dst, s)
	}
	return dst
}

func mergeUniqueUpper(dst, src []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, d := range dst {
		seen[strings.ToUpper(d)] = true
	}
	for _, s := range src {
		k := strings.ToUpper(strings.TrimSpace(s))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		dst = append(dst, k)
	}
	return dst
}
