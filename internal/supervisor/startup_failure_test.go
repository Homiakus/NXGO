package supervisor

import "testing"

func TestStartupOutputIndicatesFailure(t *testing.T) {
	if !startupOutputIndicatesFailure("journal output\nRuntime error:\nNXOpen failure") {
		t.Fatal("runtime error output must fail startup fast")
	}
	if startupOutputIndicatesFailure("NXGO bootstrap: entered\nloading dependencies\n") {
		t.Fatal("ordinary startup output must not fail startup")
	}
}
