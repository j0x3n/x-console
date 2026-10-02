//go:build !linux && !darwin && !windows

package presence

import "context"

func Get(context.Context) Sample { return Sample{} }
