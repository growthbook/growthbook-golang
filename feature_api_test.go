package growthbook

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJsonUnmarshaling(t *testing.T) {
	apiJson := `{
      "features": {
        "foo": {
          "defaultValue": "api"
        }
      },
      "experiments": [],
      "dateUpdated": "2000-05-01T00:00:12Z"
    }`
	var apiResp FeatureApiResponse
	err := json.Unmarshal([]byte(apiJson), &apiResp)
	require.Nil(t, err)
	require.Equal(t,
		FeatureApiResponse{
			Features:    FeatureMap{"foo": &Feature{DefaultValue: "api"}},
			DateUpdated: time.Date(2000, time.May, 1, 0, 0, 12, 0, time.UTC),
		},
		apiResp)
}
