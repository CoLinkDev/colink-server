package pkg

import (
	"errors"
	"testing"
)

func TestAppErrorUnwrap(t *testing.T) {
	root := errors.New("root")
	appErr := WrapError(root, 500, CodeInternalError, "internal error")

	if !errors.Is(appErr, root) {
		t.Fatal("expected wrapped error to participate in errors.Is")
	}
}
