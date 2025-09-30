package engine_eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/features"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/segments"
)

// MapEnvironmentDocumentToEvaluationContext maps an environment document model
// to the higher-level EngineEvaluationContext representation used for evaluation.
func MapEnvironmentDocumentToEvaluationContext(env *environments.EnvironmentModel) EngineEvaluationContext {
	ctx := EngineEvaluationContext{}

	// Environment
	// map environment -> EnvironmentContext
	ctx.Environment = EnvironmentContext{
		Key:  env.APIKey,
		Name: env.APIKey, // Default to APIKey, will be overridden below if project exists
	}
	if env.Project != nil {
		ctx.Environment.Name = env.Project.Name
	}

	// Features (environment defaults)
	if len(env.FeatureStates) > 0 {
		ctx.Features = make(map[string]FeatureContext, len(env.FeatureStates))
		for _, fs := range env.FeatureStates {
			fc := mapFeatureStateToFeatureContext(fs)
			ctx.Features[fc.Name] = fc
		}
	}

	// Segments (from project)
	if env.Project != nil && len(env.Project.Segments) > 0 {
		ctx.Segments = make(map[string]SegmentContext, len(env.Project.Segments))
		for _, s := range env.Project.Segments {
			sc := mapSegmentToSegmentContext(s)
			ctx.Segments[sc.Key] = sc
		}
	}

	// Identity overrides (mapped to segments)
	if len(env.IdentityOverrides) > 0 {
		identitySegments := mapIdentityOverridesToSegments(env.IdentityOverrides)
		if ctx.Segments == nil {
			ctx.Segments = make(map[string]SegmentContext)
		}
		for key, segment := range identitySegments {
			ctx.Segments[key] = segment
		}
	}

	return ctx
}

func mapFeatureStateToFeatureContext(fs *features.FeatureStateModel) FeatureContext {
	var key string
	if fs.DjangoID != 0 {
		key = strconv.Itoa(fs.DjangoID)
	} else {
		key = fs.FeatureStateUUID
	}

	fc := FeatureContext{
		Enabled:    fs.Enabled,
		FeatureKey: strconv.Itoa(fs.Feature.ID),
		Key:        key,
		Name:       fs.Feature.Name,
	}

	// Value
	if fs.RawValue != nil {
		valueStr := fmt.Sprint(fs.RawValue)
		fc.Value = &Value{String: &valueStr}
	}

	// Variants
	if len(fs.MultivariateFeatureStateValues) > 0 {
		variants := make([]FeatureValue, 0, len(fs.MultivariateFeatureStateValues))
		for _, mv := range fs.MultivariateFeatureStateValues {
			valueStr := fmt.Sprint(mv.MultivariateFeatureOption.Value)
			variants = append(variants, FeatureValue{
				Value:  &Value{String: &valueStr},
				Weight: mv.PercentageAllocation,
			})
		}
		fc.Variants = variants
	}

	// Priority (if present via segment override)
	if fs.FeatureSegment != nil {
		p := float64(fs.FeatureSegment.Priority)
		fc.Priority = &p
	}

	return fc
}

func mapSegmentToSegmentContext(s *segments.SegmentModel) SegmentContext {
	sc := SegmentContext{
		Key:   strconv.Itoa(s.ID),
		Name:  s.Name,
		Rules: make([]SegmentRule, 0, len(s.Rules)),
	}

	// Overrides
	if len(s.FeatureStates) > 0 {
		for _, fs := range s.FeatureStates {
			sc.Overrides = append(sc.Overrides, mapFeatureStateToFeatureContext(fs))
		}
	}

	// Rules
	for _, r := range s.Rules {
		sc.Rules = append(sc.Rules, mapSegmentRuleToRule(r))
	}

	return sc
}

func mapSegmentRuleToRule(r *segments.SegmentRuleModel) SegmentRule {
	er := SegmentRule{Type: mapRuleType(r.Type)}
	// Conditions
	if len(r.Conditions) > 0 {
		for _, c := range r.Conditions {
			er.Conditions = append(er.Conditions, Condition{
				Operator: mapConditionOperator(c.Operator),
				Property: c.Property,
				Value:    &ValueUnion{String: &c.Value},
			})
		}
	}
	// Nested rules
	if len(r.Rules) > 0 {
		for _, sr := range r.Rules {
			er.Rules = append(er.Rules, mapSegmentRuleToRule(sr))
		}
	}
	return er
}

func mapRuleType(t segments.RuleType) Type {
	switch t {
	case segments.All:
		return All
	case segments.Any:
		return Any
	default:
		return None
	}
}

func mapConditionOperator(op segments.ConditionOperator) Operator {
	// Normalise NOT EQUAL -> NOT_EQUAL
	if op == "NOT EQUAL" {
		return NotEqual
	}
	return Operator(op)
}

// overridesKey represents a unique set of feature overrides for grouping identities.
type overridesKey struct {
	featureKey   string
	featureName  string
	enabled      bool
	featureValue string
}

// overridesKeyList is a sortable slice of overridesKey.
type overridesKeyList []overridesKey

func (o overridesKeyList) Len() int           { return len(o) }
func (o overridesKeyList) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }
func (o overridesKeyList) Less(i, j int) bool { return o[i].featureName < o[j].featureName }

// generateHash creates a hash from the overrides key for use as segment key.
func generateHash(overrides overridesKeyList) string {
	// Sort to ensure consistent hash for same set of overrides
	sort.Sort(overrides)

	// Create a string representation of the overrides
	var hashInput string
	for _, override := range overrides {
		hashInput += fmt.Sprintf("%s:%s:%t:%s;", override.featureKey, override.featureName, override.enabled, override.featureValue)
	}

	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:])[:16] // Use first 16 characters for shorter key
}

// This groups identities by their common feature overrides and creates segments for each group.
func mapIdentityOverridesToSegments(identityOverrides []*identities.IdentityModel) map[string]SegmentContext {
	// Map from overrides key to list of identifiers
	featuresToIdentifiers := make(map[string][]string)
	overridesKeyToList := make(map[string]overridesKeyList)

	for _, identityOverride := range identityOverrides {
		if len(identityOverride.IdentityFeatures) == 0 {
			continue
		}

		// Create overrides key from sorted features
		var overrides overridesKeyList
		for _, featureState := range identityOverride.IdentityFeatures {
			featureValue := ""
			if featureState.RawValue != nil {
				featureValue = fmt.Sprint(featureState.RawValue)
			}

			overrides = append(overrides, overridesKey{
				featureKey:   strconv.Itoa(featureState.Feature.ID),
				featureName:  featureState.Feature.Name,
				enabled:      featureState.Enabled,
				featureValue: featureValue,
			})
		}

		// Generate hash for this set of overrides
		overridesHash := generateHash(overrides)

		// Group identifiers by their overrides
		featuresToIdentifiers[overridesHash] = append(featuresToIdentifiers[overridesHash], identityOverride.Identifier)
		overridesKeyToList[overridesHash] = overrides
	}

	// Create segment contexts for each unique set of overrides
	segmentContexts := make(map[string]SegmentContext)

	for overridesHash, identifiers := range featuresToIdentifiers {
		overrides := overridesKeyToList[overridesHash]

		// Create segment context
		sc := SegmentContext{
			Key:  "", // Identity override segments never use % Split operator
			Name: "identity_overrides",
			Rules: []SegmentRule{
				{
					Type: All,
					Conditions: []Condition{
						{
							Operator: "IN",
							Property: "$.identity.identifier",
							Value:    &ValueUnion{String: func() *string { s := strings.Join(identifiers, ","); return &s }()},
						},
					},
				},
			},
		}

		// Create overrides for each feature
		for _, override := range overrides {
			priority := math.Inf(-1) // Highest possible priority
			featureOverride := FeatureContext{
				Key:        "", // Identity overrides never carry multivariate options
				FeatureKey: override.featureKey,
				Name:       override.featureName,
				Enabled:    override.enabled,
				Priority:   &priority,
			}

			// Set the value if provided
			if override.featureValue != "" {
				featureOverride.Value = &Value{String: &override.featureValue}
			}

			sc.Overrides = append(sc.Overrides, featureOverride)
		}

		segmentContexts[overridesHash] = sc
	}

	return segmentContexts
}

// Trait represents a trait with key-value pair, compatible with the main package Trait struct.
type Trait struct {
	TraitKey   string      `json:"trait_key"`
	TraitValue interface{} `json:"trait_value"`
	Transient  bool        `json:"transient,omitempty"`
}

// MapContextAndIdentityDataToContext maps context and identity data to create an evaluation context
// with identity information. This function takes an existing context and enriches it with identity
// data including identifier and traits.
func MapContextAndIdentityDataToContext(
	context EngineEvaluationContext,
	identifier string,
	traits interface{},
) EngineEvaluationContext {
	// Convert traits to local type
	var traitList []*Trait

	if traits != nil {
		// Handle different trait types by copying field values
		switch v := traits.(type) {
		case []*Trait:
			traitList = v
		default:
			// Try to extract traits using reflection-like approach
			// Since both Trait structs have the same JSON tags, we can marshal/unmarshal
			if jsonBytes, err := json.Marshal(traits); err == nil {
				if err := json.Unmarshal(jsonBytes, &traitList); err != nil {
					// Log error or handle gracefully - for now, continue with empty list
					traitList = nil
				}
			}
		}
	}
	// Create a copy of the context
	newContext := context

	// Create traits map for the identity
	identityTraits := make(map[string]*Value)

	for _, trait := range traitList {
		if trait == nil {
			continue
		}

		// Convert trait value to *Value
		valuePtr := convertTraitValueToValue(trait.TraitValue)
		if valuePtr != nil {
			identityTraits[trait.TraitKey] = valuePtr
		}
	}

	// Create the identity context
	var environmentKey string
	if newContext.Environment.Key != "" {
		environmentKey = newContext.Environment.Key
	} else {
		environmentKey = newContext.Environment.Name
	}

	identity := IdentityContext{
		Identifier: identifier,
		Key:        fmt.Sprintf("%s_%s", environmentKey, identifier),
		Traits:     identityTraits,
	}

	// Set the identity in the context
	newContext.Identity = &identity

	return newContext
}

// This function handles interface{} values and converts them appropriately.
func convertTraitValueToValue(traitValue interface{}) *Value {
	if traitValue == nil {
		return nil
	}

	switch v := traitValue.(type) {
	case bool:
		return &Value{Bool: &v}
	case int:
		f := float64(v)
		return &Value{Double: &f}
	case int64:
		f := float64(v)
		return &Value{Double: &f}
	case float64:
		return &Value{Double: &v}
	case float32:
		f := float64(v)
		return &Value{Double: &f}
	case string:
		if v == "" {
			return nil
		}
		// Try to parse string as boolean
		if v == "true" {
			b := true
			return &Value{Bool: &b}
		} else if v == "false" {
			b := false
			return &Value{Bool: &b}
		}
		// Try to parse string as float64
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return &Value{Double: &f}
		}
		// Default to string
		return &Value{String: &v}
	default:
		// For other types, convert to string
		str := fmt.Sprint(v)
		if str == "" {
			return nil
		}
		return &Value{String: &str}
	}
}
