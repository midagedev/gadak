package fields

import (
	"sort"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// Discover groups the custom-field catalog by normalized display name, keeps
// the groups the mirror has actually seen filled, and returns specs ordered by
// total fill count descending. Prior Auto:false specs are preserved; Auto:true
// prior entries donate aliases for stability.
func Discover(catalog []jira.FieldInfo, fill map[string]int, prior []config.FieldSpec) []config.FieldSpec {
	reservedAlias := map[string]bool{}
	reservedID := map[string]bool{}
	var fixed []config.FieldSpec

	// prior auto entries: reuse alias when label matches or IDs overlap.
	priorAliasByNormLabel := map[string]string{}
	priorAliasByID := map[string]string{}

	for _, p := range prior {
		if !p.Auto {
			// Deep-copy so callers can mutate freely.
			cp := p
			if p.IDs != nil {
				cp.IDs = append([]string(nil), p.IDs...)
			}
			fixed = append(fixed, cp)
			if p.Alias != "" {
				reservedAlias[p.Alias] = true
			}
			for _, id := range p.IDs {
				if id != "" {
					reservedID[id] = true
				}
			}
			continue
		}
		if p.Alias == "" {
			continue
		}
		if p.Label != "" {
			priorAliasByNormLabel[NormalizeName(p.Label)] = p.Alias
		}
		for _, id := range p.IDs {
			if id != "" {
				priorAliasByID[id] = p.Alias
			}
		}
	}

	type member struct {
		info jira.FieldInfo
		fill int
	}
	groups := map[string][]member{} // norm name → members
	var groupOrder []string
	for _, f := range catalog {
		if !f.Custom {
			continue
		}
		if reservedID[f.ID] {
			continue
		}
		norm := NormalizeName(f.Name)
		if norm == "" {
			norm = f.ID
		}
		if _, ok := groups[norm]; !ok {
			groupOrder = append(groupOrder, norm)
		}
		groups[norm] = append(groups[norm], member{info: f, fill: fill[f.ID]})
	}

	type candidate struct {
		spec config.FieldSpec
		fill int
	}
	var cands []candidate
	usedAliases := map[string]bool{}
	for a := range reservedAlias {
		usedAliases[a] = true
	}

	for _, norm := range groupOrder {
		members := groups[norm]
		total := 0
		for _, m := range members {
			total += m.fill
		}
		if total < 1 {
			continue
		}
		// Sort ids: fill desc, then id asc.
		sort.SliceStable(members, func(i, j int) bool {
			if members[i].fill != members[j].fill {
				return members[i].fill > members[j].fill
			}
			return members[i].info.ID < members[j].info.ID
		})
		primary := members[0].info
		role, kind, ok := Classify(primary)
		if !ok {
			continue
		}
		ids := make([]string, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.info.ID)
		}
		label := primary.Name
		alias := ""
		// Prefer prior alias by overlapping id, then by normalized label. A prior
		// spec whose ids split into two groups this round must donate its alias
		// to only one of them, hence the usedAliases guard.
		for _, id := range ids {
			if a := priorAliasByID[id]; a != "" && !reservedAlias[a] && !usedAliases[a] {
				alias = a
				break
			}
		}
		if alias == "" {
			if a := priorAliasByNormLabel[norm]; a != "" && !reservedAlias[a] && !usedAliases[a] {
				alias = a
			}
		}
		if alias == "" {
			alias = SuggestAlias(label, primary.ID, usedAliases)
		}
		usedAliases[alias] = true
		cands = append(cands, candidate{
			spec: config.FieldSpec{
				Alias: alias,
				Label: label,
				IDs:   ids,
				Role:  role,
				Kind:  kind,
				Auto:  true,
			},
			fill: total,
		})
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].fill != cands[j].fill {
			return cands[i].fill > cands[j].fill
		}
		return cands[i].spec.Alias < cands[j].spec.Alias
	})

	out := make([]config.FieldSpec, 0, len(fixed)+len(cands))
	out = append(out, fixed...)
	for _, c := range cands {
		out = append(out, c.spec)
	}
	return out
}
