package osinfo

import (
	"testing"

	require "github.com/stretchr/testify/require"
)

func Test__Name(t *testing.T) {
	// TBH, it is hard to write a test for this that would work on
	// all environments.
	// The only test I can think of is that the returned string is not empty,
	// and that it doesn't crash.

	name := Name()
	require.NotEmpty(t, name)
}
