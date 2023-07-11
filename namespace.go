package growthbook

import (
	"encoding/json"
	"errors"
)

// Namespace specifies what part of a namespace an experiment
// includes. If two experiments are in the same namespace and their
// ranges don't overlap, they wil be mutually exclusive.
type Namespace struct {
	ID    string
	Start float64
	End   float64
}

func (ns *Namespace) Copy() *Namespace {
	return &Namespace{
		ID:    ns.ID,
		Start: ns.Start,
		End:   ns.End,
	}
}

func (ns *Namespace) MarshalJSON() ([]byte, error) {
	return json.Marshal([]interface{}{ns.ID, ns.Start, ns.End})
}

func (ns *Namespace) UnmarshalJSON(b []byte) error {
	tmp := []interface{}{&ns.ID, &ns.Start, &ns.End}
	okLen := len(tmp)
	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}
	if len(tmp) != okLen {
		return errors.New("Wrong number of JSON fields for namespace")
	}
	return nil
}

// Determine whether a user's ID lies within a given namespace.
func (ns *Namespace) inNamespace(userID string) bool {
	n := float64(hashFnv32a(userID+"__"+ns.ID)%1000) / 1000
	return n >= ns.Start && n < ns.End
}
