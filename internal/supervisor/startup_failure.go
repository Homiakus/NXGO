package supervisor

import "strings"

// startupOutputIndicatesFailure recognizes the stable run_journal diagnostic
// emitted when a journal raises an exception while the wrapper process itself
// is still alive and therefore cannot be observed through Cmd.Wait yet.
func startupOutputIndicatesFailure(output string) bool {
	return strings.Contains(output, "Runtime error:")
}
