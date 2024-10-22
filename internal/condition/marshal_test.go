package condition

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBaseEmptyLogical(t *testing.T) {
	j := []byte(`{"$and": [], "$or": [], "$nor": [], "$not": {"$and": []}}`)
	var b Base
	err := json.Unmarshal(j, &b)
	require.Nil(t, err)
	require.Equal(t, Base{And{}, Or{}, Nor{}, Not{And{}}}, b)
}
