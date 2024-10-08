package growthbook

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGetPath(t *testing.T) {
	attrs := Attributes{
		"userId": "10",
		"user": Attributes{
			"name":    "Bob",
			"country": "UK",
			"tags":    []string{"user", "uk", "new"},
		}}
	t.Run("field not found", func(t *testing.T) {
		res := getPath("noway", attrs)
		require.Nil(t, res)
	})

	t.Run("field on top level", func(t *testing.T) {
		res := getPath("userId", attrs)
		require.Equal(t, "10", res)
	})

	t.Run("field is substruct", func(t *testing.T) {
		res := getPath("user.name", attrs)
		require.Equal(t, "Bob", res)
	})
}
