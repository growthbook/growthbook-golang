package growthbook

import (
	"encoding/json"
	"fmt"
)

// Namespace specifies what part of a namespace an experiment
// includes. If two experiments are in the same namespace and their
// ranges don't overlap, they wil be mutually exclusive.
type Namespace struct {
	Id    string  `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Determine whether a user's ID lies within a given namespace.
func (namespace *Namespace) inNamespace(userId string) bool {
	n := float64(hashFnv32a(userId+"__"+namespace.Id)%1000) / 1000
	return n >= namespace.Start && n < namespace.End
}

func (namespace *Namespace) UnmarshalJSON(data []byte) error {
	arr := []any{}
	err := json.Unmarshal(data, &arr)
	if err != nil {
		return err
	}

	if len(arr) != 3 {
		return fmt.Errorf("invalid namespace format: %v", arr)
	}

	id, ok1 := arr[0].(string)
	start, ok2 := arr[1].(float64)
	end, ok3 := arr[2].(float64)

	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid namespace format: %v", arr)
	}
	namespace.Id = id
	namespace.Start = start
	namespace.End = end

	return nil
}
