package zoxide

import "github.com/Wingsdh/cc-sesh/v2/model"

func (z *RealZoxide) Query(query string) (*model.ZoxideResult, error) {
	parts, err := z.shell.PrepareCmd(z.queryCommand, map[string]string{"{}": query})
	if err != nil {
		return nil, err
	}
	result, err := z.shell.Cmd(parts[0], parts[1:]...)
	// TODO: handle no result found
	if err != nil {
		return nil, err
	}
	return &model.ZoxideResult{
		Score: 0,
		Path:  result,
	}, nil
}
