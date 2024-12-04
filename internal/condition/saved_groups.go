package condition

import (
	"encoding/json"

	"github.com/growthbook/growthbook-golang/internal/value"
)

type SavedGroups map[string]value.ArrValue

func (sg *SavedGroups) UnmarshalJSON(data []byte) error {
	var groups map[string][]any
	if err := json.Unmarshal(data, &groups); err != nil {
		return err
	}
	*sg = SavedGroups{}
	for k, v := range groups {
		vv := value.New(v)
		if arr, ok := vv.(value.ArrValue); ok {
			(*sg)[k] = arr
		}
	}
	return nil
}
