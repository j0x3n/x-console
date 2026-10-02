package presence

import (
	"context"
	"os"
)

func Get(ctx context.Context) Sample {
	return sampleLinux(ctx, runCommand, os.Getenv("XDG_SESSION_ID"))
}
