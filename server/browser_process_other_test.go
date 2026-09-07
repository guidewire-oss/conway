//go:build !darwin && !linux

package main

import (
	"context"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
)

func browserCommand(context.Context, string, ...string) *exec.Cmd {
	Skip("Browser acceptance process cleanup is supported on macOS and Linux.")
	return nil
}
