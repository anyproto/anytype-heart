package debug

import "context"

func (d *debug) NetCheck(ctx context.Context, clientYml string) (string, error) {
	return "offline", nil
}
