package linear

// StatusCategory maps a WorkflowState.type onto gadak's status_category
// contract (new | inprogress | done). The enum is open — Linear added
// "duplicate" after the original six — so ok reports whether the type was
// known; unknown collapses to new (an issue can only be misread as open,
// never as silently done). Never key on state names: they are display text
// ("진행 중").
//
// This is the single owner of the Linear type→category collapse (GDK-665).
// The read path (internal/sync) and the write path (origin.linearWriter)
// both call it. The write path then maps the gadak token through
// statuscat.CategoryKey onto the Jira REST statusCategory key that
// origin.Transition.To carries, because that is the shape jira.Client
// unmarshals and the HTTP handler already runs through statuscat.Category.
func StatusCategory(stateType string) (cat string, ok bool) {
	switch stateType {
	case "started":
		return "inprogress", true
	case "unstarted", "backlog", "triage":
		return "new", true
	case "completed", "canceled", "duplicate":
		return "done", true
	}
	return "new", false
}
