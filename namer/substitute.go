package namer

import (
	"strings"

	"github.com/Wingsdh/cc-sesh/v2/model"
)

// applySubstitutions runs every rule against name in order, feeding each rule
// the output of the one before it, and returns the result. Each rule replaces
// all occurrences of Find with Replace. A rule with an empty Find is skipped:
// the config loader rejects it up front, so this only guards against
// unexpected input.
func applySubstitutions(name string, rules []model.NameSubstitution) string {
	for _, rule := range rules {
		if rule.Find == "" {
			continue
		}
		name = strings.ReplaceAll(name, rule.Find, rule.Replace)
	}
	return name
}

// nameSubstitution is a naming strategy that applies the user's
// [[name_substitution]] rules to the home-collapsed path. It returns a name
// only when the rules actually change the path; otherwise it returns an empty
// string so the namer falls through to the git and directory strategies. This
// keeps naming unchanged for paths no rule matches.
func nameSubstitution(n *RealNamer, path string) (string, error) {
	if len(n.config.NameSubstitutions) == 0 {
		return "", nil
	}
	shortened, err := n.home.ShortenHome(path)
	if err != nil {
		return "", err
	}
	name := applySubstitutions(shortened, n.config.NameSubstitutions)
	if name == shortened {
		return "", nil
	}
	return name, nil
}
