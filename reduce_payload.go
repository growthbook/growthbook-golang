package growthbook

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
)

// ErrNoPayloadJSON is returned by Client.ReducePayload when the client's
// features were set from Go values rather than from a JSON payload, so there
// is no payload to reduce.
var ErrNoPayloadJSON = errors.New("no JSON features payload loaded")

// PayloadFilters selects features and auto experiments by their payload
// metadata. Each filter is a list, and any one of its values is enough. A nil
// list doesn't filter; an empty one matches nothing.
type PayloadFilters struct {
	Projects     []string
	Tags         []string
	CustomFields map[string][]any
}

// PrerequisiteMode says how ReducePayload treats the prerequisites of what it
// keeps.
type PrerequisiteMode string

const (
	// PrerequisitesKeep (the default) sends the prerequisites of what is kept
	// even when the filters remove them, so the client evaluates the same as
	// with the full payload.
	PrerequisitesKeep PrerequisiteMode = "keep"
	// PrerequisitesFilter applies the filters to prerequisites too, except
	// holdouts, which aren't app features and are always kept.
	PrerequisitesFilter PrerequisiteMode = "filter"
)

// ReducePayloadOptions configures ReducePayload.
type ReducePayloadOptions struct {
	// Include keeps only the features and auto experiments that match every filter.
	Include PayloadFilters
	// Exclude removes the features and auto experiments that match any filter.
	Exclude PayloadFilters
	// Prerequisites defaults to PrerequisitesKeep.
	Prerequisites PrerequisiteMode
}

// PruneCondition partially evaluates a condition, decoded from JSON, when only
// some attributes are known, e.g. on a server. Attributes missing from
// attributes are unknown. It returns a bool once the result can't depend on
// them, otherwise the condition (a map[string]any) minus every branch that is
// already decided. savedGroups is the payload's decoded savedGroups section.
func PruneCondition(cond map[string]any, attributes Attributes, savedGroups map[string]any) (any, error) {
	groups, err := condition.SavedGroups(savedGroups).Normalize()
	if err != nil {
		return nil, err
	}
	return condition.Prune(attributesValue(attributes), cond, groups), nil
}

// ReducePayload shrinks a decrypted features payload, the JSON body of the
// GrowthBook features API, for a user whose attributes are only partly known,
// e.g. on a server bootstrapping a client SDK. The result evaluates the same as
// the full payload for anyone who agrees with the known attributes, and keeps
// the payload's other keys.
func ReducePayload(payload []byte, attributes Attributes, opts ReducePayloadOptions) ([]byte, error) {
	var p map[string]any
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}
	reduced, err := reducePayload(p, attributesValue(attributes), opts)
	if err != nil {
		return nil, err
	}
	return json.Marshal(reduced)
}

// ReducePayload reduces the client's current features payload for the
// client's attributes. See the package-level ReducePayload. It returns
// ErrNoPayloadJSON when the client's features were set from Go values.
func (client *Client) ReducePayload(opts ReducePayloadOptions) ([]byte, error) {
	payload := client.data.getPayloadJSON()
	if payload == nil {
		return nil, ErrNoPayloadJSON
	}
	reduced, err := reducePayload(payload, client.attributes, opts)
	if err != nil {
		return nil, err
	}
	return json.Marshal(reduced)
}

func attributesValue(attributes Attributes) value.ObjValue {
	obj, _ := value.New(map[string]any(attributes)).(value.ObjValue)
	if obj == nil {
		obj = value.ObjValue{}
	}
	return obj
}

func reducePayload(payload map[string]any, attrs value.ObjValue, opts ReducePayloadOptions) (map[string]any, error) {
	rawGroups, _ := payload["savedGroups"].(map[string]any)
	groups, err := condition.SavedGroups(rawGroups).Normalize()
	if err != nil {
		return nil, err
	}
	allFeatures, _ := payload["features"].(map[string]any)
	allExperiments, _ := payload["experiments"].([]any)
	include, exclude := opts.Include, opts.Exclude

	// A rule without projects applies wherever its feature is delivered
	ruleInProjects := func(meta map[string]any) bool {
		return include.Projects == nil || meta == nil || meta["projects"] == nil ||
			has(meta["projects"], include.Projects)
	}
	// A feature or experiment has to list one of the projects, or be in all of
	// them. One without projects is in none, like one without tags.
	inProjects := func(meta map[string]any) bool {
		return include.Projects == nil || truthy(meta["allProjects"]) ||
			has(meta["projects"], include.Projects)
	}
	// Kept when it matches every include filter and no exclude filter. Only
	// listed projects are excluded, so one in all projects stays.
	matches := func(meta map[string]any) bool {
		fields, _ := meta["customFields"].(map[string]any)
		if !inProjects(meta) || (include.Tags != nil && !has(meta["tags"], include.Tags)) {
			return false
		}
		for k, want := range include.CustomFields {
			if !hasAny(fields[k], want) {
				return false
			}
		}
		if has(meta["projects"], exclude.Projects) || has(meta["tags"], exclude.Tags) {
			return false
		}
		for k, want := range exclude.CustomFields {
			if hasAny(fields[k], want) {
				return false
			}
		}
		return true
	}

	// Reduces a rule or an auto experiment
	prune := func(item any) any {
		obj, ok := item.(map[string]any)
		if !ok {
			return item
		}
		obj = shallowCopy(obj)
		cond, ok := obj["condition"].(map[string]any)
		if !ok {
			return obj
		}
		switch r := condition.Prune(attrs, cond, groups).(type) {
		case bool:
			if r {
				delete(obj, "condition")
				break
			}
			// Kept, since sticky buckets and forced variations skip an
			// experiment's condition, and a rule's prerequisites are checked
			// before it
			obj["condition"] = map[string]any{"$not": map[string]any{}}
			// A force rule that never passes never returns its value
			if _, ok := obj["force"]; ok {
				obj["force"] = nil
			}
		default:
			obj["condition"] = r
		}
		return obj
	}

	// Adds a feature, then the prerequisites the client evaluates for it, even
	// when the filters drop them. With PrerequisitesFilter, only the filters
	// decide, except for holdouts.
	features := map[string]any{}
	var add func(id string)
	addParents := func(item any) {
		obj, _ := item.(map[string]any)
		parents, _ := obj["parentConditions"].([]any)
		for _, p := range parents {
			parent, _ := p.(map[string]any)
			id, ok := parent["id"].(string)
			if ok && (opts.Prerequisites != PrerequisitesFilter || strings.HasPrefix(id, "$holdout:")) {
				add(id)
			}
		}
	}
	add = func(id string) {
		feature, ok := allFeatures[id].(map[string]any)
		if _, added := features[id]; !ok || added {
			return
		}
		// Rules scoped to other projects aren't served. A prerequisite from
		// another project keeps its own, as on the server.
		meta, _ := feature["metadata"].(map[string]any)
		scoped := inProjects(meta)
		rules, ok := feature["rules"].([]any)
		if !ok {
			features[id] = feature
			return
		}
		kept := []any{}
		for _, rule := range rules {
			ruleObj, _ := rule.(map[string]any)
			ruleMeta, _ := ruleObj["metadata"].(map[string]any)
			if !scoped || ruleInProjects(ruleMeta) {
				kept = append(kept, prune(rule))
			}
		}
		feature = shallowCopy(feature)
		feature["rules"] = kept
		// Stored before its parents are added, so a prerequisite cycle ends
		features[id] = feature
		for _, rule := range kept {
			addParents(rule)
		}
	}
	for id, f := range allFeatures {
		obj, _ := f.(map[string]any)
		meta, _ := obj["metadata"].(map[string]any)
		if matches(meta) {
			add(id)
		}
	}
	experiments := []any{}
	for _, e := range allExperiments {
		obj, _ := e.(map[string]any)
		meta, _ := obj["metadata"].(map[string]any)
		if matches(meta) {
			experiments = append(experiments, prune(e))
		}
	}
	for _, e := range experiments {
		addParents(e)
	}

	// The contextual bandits that kept rules and experiments use. A context
	// whose condition never passes can't be the first that does, so it goes.
	allBandits, banditsPresent := payload["contextualBandits"].(map[string]any)
	bandits := map[string]any{}
	items := append([]any{}, experiments...)
	for _, f := range features {
		obj, _ := f.(map[string]any)
		rules, _ := obj["rules"].([]any)
		items = append(items, rules...)
	}
	for _, item := range items {
		obj, _ := item.(map[string]any)
		ref, _ := obj["contextualBanditRef"].(string)
		if _, done := bandits[ref]; ref == "" || done {
			continue
		}
		bandit, ok := allBandits[ref].(map[string]any)
		if !ok {
			continue
		}
		contexts, _ := bandit["contexts"].([]any)
		keptContexts := []any{}
		for _, c := range contexts {
			ctxObj, ok := c.(map[string]any)
			if !ok {
				keptContexts = append(keptContexts, c)
				continue
			}
			cond, _ := ctxObj["condition"].(map[string]any)
			if cond == nil {
				cond = map[string]any{}
			}
			r := condition.Prune(attrs, cond, groups)
			if r == false {
				continue
			}
			if r == true {
				r = map[string]any{}
			}
			ctxObj = shallowCopy(ctxObj)
			ctxObj["condition"] = r
			keptContexts = append(keptContexts, ctxObj)
		}
		bandit = shallowCopy(bandit)
		bandit["contexts"] = keptContexts
		bandits[ref] = bandit
	}

	// Keep each saved group whose id appears as a string anywhere, with
	// condition groups reduced. This needs no list of the operators that
	// reference groups.
	keptGroups := map[string]any{}
	var keep func(x any)
	keep = func(x any) {
		switch v := x.(type) {
		case string:
			entry, ok := rawGroups[v]
			if _, done := keptGroups[v]; !ok || done {
				return
			}
			if cond, ok := condition.PruneSavedGroup(attrs, v, groups).(map[string]any); ok {
				if obj, isObj := entry.(map[string]any); isObj {
					obj = shallowCopy(obj)
					obj["condition"] = cond
					entry = obj
				}
			}
			keptGroups[v] = entry
			keep(entry)
		case map[string]any:
			for _, e := range v {
				keep(e)
			}
		case []any:
			for _, e := range v {
				keep(e)
			}
		}
	}
	keep([]any{features, experiments, bandits})

	out := shallowCopy(payload)
	out["features"] = features
	out["experiments"] = experiments
	out["savedGroups"] = keptGroups
	// Only when there were any, as with the payload's other optional keys
	if banditsPresent {
		out["contextualBandits"] = bandits
	} else {
		delete(out, "contextualBandits")
	}
	return out, nil
}

// has reports whether a metadata value shares a value with want, like $in:
// an array needs any one of its values in want. A nil want matches nothing.
func has(actual any, want []string) bool {
	if want == nil {
		return false
	}
	list := make([]any, len(want))
	for i, w := range want {
		list[i] = w
	}
	return hasAny(actual, list)
}

func hasAny(actual any, want []any) bool {
	if want == nil {
		return false
	}
	if arr, ok := actual.([]any); ok {
		for _, el := range arr {
			if includes(want, el) {
				return true
			}
		}
		return false
	}
	return includes(want, actual)
}

// includes compares like JS Array.prototype.includes: primitives by value,
// arrays and objects never equal.
func includes(list []any, x any) bool {
	for _, el := range list {
		switch e := el.(type) {
		case string, bool, nil:
			if e == x {
				return true
			}
		case float64:
			if n, ok := x.(float64); ok && n == e {
				return true
			}
		case int:
			if n, ok := x.(float64); ok && n == float64(e) {
				return true
			}
		}
	}
	return false
}

func shallowCopy(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
