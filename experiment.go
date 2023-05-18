package growthbook

// NewExperiment creates an experiment with default settings: active,
// but all other fields empty.
func NewExperiment(key string) *Experiment {
	return &Experiment{
		Key:    key,
		Active: true,
	}
}

// WithVariations set the feature variations for an experiment.
func (exp *Experiment) WithVariations(variations ...FeatureValue) *Experiment {
	exp.Variations = variations
	return exp
}

// WithRanges set the ranges for an experiment.
func (exp *Experiment) WithRanges(ranges ...Range) *Experiment {
	exp.Ranges = ranges
	return exp
}

// WithMeta sets the meta information for an experiment.
func (exp *Experiment) WithMeta(meta ...VariationMeta) *Experiment {
	exp.Meta = meta
	return exp
}

// WithWeights set the weights for an experiment.
func (exp *Experiment) WithWeights(weights ...float64) *Experiment {
	exp.Weights = weights
	return exp
}

// WithSeed sets the hash seed for an experiment.
func (exp *Experiment) WithSeed(seed string) *Experiment {
	exp.Seed = seed
	return exp
}

// WithName sets the name for an experiment.
func (exp *Experiment) WithName(name string) *Experiment {
	exp.Name = name
	return exp
}

// WithPhase sets the phase for an experiment.
func (exp *Experiment) WithPhase(phase string) *Experiment {
	exp.Phase = phase
	return exp
}

// WithActive sets the enabled flag for an experiment.
func (exp *Experiment) WithActive(active bool) *Experiment {
	exp.Active = active
	return exp
}

// WithCoverage sets the coverage for an experiment.
func (exp *Experiment) WithCoverage(coverage float64) *Experiment {
	exp.Coverage = &coverage
	return exp
}

// WithCondition sets the condition for an experiment.
func (exp *Experiment) WithCondition(condition Condition) *Experiment {
	exp.Condition = condition
	return exp
}

// WithNamespace sets the namespace for an experiment.
func (exp *Experiment) WithNamespace(namespace *Namespace) *Experiment {
	exp.Namespace = namespace
	return exp
}

// WithForce sets the forced value index for an experiment.
func (exp *Experiment) WithForce(force int) *Experiment {
	exp.Force = &force
	return exp
}

// WithHashAttribute sets the hash attribute for an experiment.
func (exp *Experiment) WithHashAttribute(hashAttribute string) *Experiment {
	exp.HashAttribute = hashAttribute
	return exp
}
