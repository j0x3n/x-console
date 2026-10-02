package presence

import "context"

func Get(ctx context.Context) Sample { return sampleDarwin(ctx, runCommand) }
