package engine_eval

// Evaluation result object containing flag evaluation results, and
// segments used in the evaluation.
type EvaluationResult struct {
	// Feature flags evaluated for the context, mapped by feature names.
	Flags map[string]*FlagResult `json:"flags"`
	// List of segments which the provided context belongs to.
	Segments []SegmentResult `json:"segments"`
}

type FlagResult struct {
	// Indicates if the feature flag is enabled.
	Enabled bool `json:"enabled"`
	// Feature name.
	Name string `json:"name"`
	// Reason for the feature flag evaluation.
	Reason string `json:"reason,omitempty"`
	// Feature flag value.
	Value any `json:"value,omitempty"`
	// Metadata about the feature.
	Metadata *FeatureMetadata `json:"metadata,omitempty"`
}

type SegmentResult struct {
	// Segment name.
	Name string `json:"name"`
	// Metadata about the segment.
	Metadata *SegmentMetadata `json:"metadata,omitempty"`
}
