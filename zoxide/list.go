package zoxide

import (
	"strings"

	"github.com/Wingsdh/cc-sesh/v2/convert"
	"github.com/Wingsdh/cc-sesh/v2/model"
)

func (z *RealZoxide) ListResults() ([]*model.ZoxideResult, error) {
	parts, err := z.shell.PrepareCmd(z.listCommand, map[string]string{})
	if err != nil {
		return nil, err
	}
	list, err := z.shell.ListCmd(parts[0], parts[1:]...)
	if err != nil {
		return nil, err
	}
	results := make([]*model.ZoxideResult, 0, len(list))
	for _, result := range list {
		line := strings.TrimSpace(result)
		if line == "" {
			continue
		}
		results = append(results, parseResult(line))
	}
	return results, nil
}

// parseResult auto-detects an optional leading numeric score. When the first
// space-delimited field parses as a float it is the score and the rest is the
// path (zoxide's `--score` output); otherwise the whole line is the path.
func parseResult(line string) *model.ZoxideResult {
	if fields := strings.SplitN(line, " ", 2); len(fields) == 2 {
		if score, err := convert.StringToFloat(fields[0]); err == nil {
			return &model.ZoxideResult{Score: score, Path: fields[1]}
		}
	}
	return &model.ZoxideResult{Score: 0, Path: line}
}
