package namer

import (
	"regexp"
	"strings"

	"github.com/Wingsdh/cc-sesh/v2/model"
)

// applySubstitutions runs every rule against name in order, feeding each rule
// the output of the one before it, and returns the result. A literal rule
// replaces all occurrences of Find; a regex rule replaces every match and may
// use $1-style references to capture groups. A rule with an empty Find or an
// invalid regex is skipped so a single bad rule can't break naming outright:
// the config loader rejects both up front, so this only guards against
// unexpected input.
func applySubstitutions(name string, rules []model.NameSubstitution) string {
	for _, rule := range rules {
		if rule.Find == "" {
			continue
		}
		if rule.Regex {
			re, err := regexp.Compile(rule.Find)
			if err != nil {
				continue
			}
			name = re.ReplaceAllString(name, rule.Replace)
			continue
		}
		name = strings.ReplaceAll(name, rule.Find, rule.Replace)
	}
	return name
}

// nameSubstitution is a naming strategy that applies the user's
// [[name_substitution]] rules to the home-collapsed path. It returns a name
// only when the rules actually change the path into something that is still a
// usable name; otherwise it returns an empty string so the namer falls through
// to the git and directory strategies. This keeps naming unchanged for paths no
// rule matches.
func nameSubstitution(n *RealNamer, path string) (string, error) {
	if len(n.config.NameSubstitutions) == 0 {
		return "", nil
	}
	// Rules are written against the "~"-collapsed path, so without a home
	// directory they can't be matched; let the later strategies name it
	// rather than failing every lookup.
	shortened, err := n.home.ShortenHome(path)
	if err != nil {
		return "", nil
	}
	name := applySubstitutions(shortened, n.config.NameSubstitutions)
	if name == shortened {
		return "", nil
	}
	// A rule that leaves only whitespace would legalize to an empty session
	// name; treat that as no name so the later strategies still get a turn.
	if convertToValidName(name) == "" {
		return "", nil
	}
	return name, nil
}
