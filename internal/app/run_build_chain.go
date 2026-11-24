package app

import (
	"context"
)

// buildBeforeLaunch runs OpenIDEA's Build Project, Build Module, Build
// Artifacts and CMake before-launch tasks to completion. The Build tool window
// receives the same streamed plan and output as an interactive build.
func (m *Model) buildBeforeLaunch(
	ctx context.Context,
	kind string,
	file *string,
	cmake *string,
	activate bool,
) error {
	action, arg, err := beforeLaunchBuildAction(kind, file, cmake, activate)
	if err != nil {
		return err
	}
	return m.runBuildActionWait(ctx, action, arg)
}
