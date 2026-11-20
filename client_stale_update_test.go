package growthbook

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConcurrentUpdatesKeepTheNewestPayload(t *testing.T) {
	// Large enough that both updates are decoding at the same time
	var flags strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&flags, `"filler-%d": {"defaultValue": %d, "rules": [{"condition": {"id": "%d"}, "force": 1}]},`, i, i, i)
	}
	payload := func(value, dateUpdated string) string {
		return `{"features": {` + flags.String() + `"flag": {"defaultValue": "` + value + `"}}, "dateUpdated": "` + dateUpdated + `"}`
	}
	older := payload("old", "2030-01-01T00:00:00Z")
	newer := payload("new", "2030-01-02T00:00:00Z")

	ctx := context.Background()
	for i := 0; i < 50; i++ {
		client, err := NewClient(ctx, withSilentTestLogger())
		require.NoError(t, err)
		var wg sync.WaitGroup
		for _, p := range []string{newer, older} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				require.NoError(t, client.UpdateFromApiResponseJSON(p))
			}()
		}
		wg.Wait()
		require.Equal(t, "new", client.EvalFeature(ctx, "flag").Value, "trial %d", i)
	}
}
