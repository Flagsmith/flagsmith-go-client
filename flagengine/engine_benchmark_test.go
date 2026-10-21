package flagengine_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tailscale/hujson"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine"
	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/engine_eval"
)

// BenchmarkGetEvaluationResult measures one evaluation per workload: the engine test data
// contexts benchmarked by flagsmith-engine, every engine test data context with and without
// flag dependencies, and a synthetic environment with and without flag dependencies.
func BenchmarkGetEvaluationResult(b *testing.B) {
	files, err := filepath.Glob(filepath.Join(TestDataDir, "*.json*"))
	require.NoError(b, err)
	require.NotEmpty(b, files)

	contexts := map[string]*engine_eval.EngineEvaluationContext{}
	for _, file := range files {
		spec, err := os.ReadFile(file)
		require.NoError(b, err)
		spec, err = hujson.Standardize(spec)
		require.NoError(b, err)
		var testCase struct {
			Context engine_eval.EngineEvaluationContext `json:"context"`
		}
		require.NoError(b, json.Unmarshal(spec, &testCase))
		contexts[filepath.Base(file)] = &testCase.Context
	}

	upstreamContexts := []*engine_eval.EngineEvaluationContext{
		contexts["test_0cfd0d72-4de4-4ed7-9cfb-d80dc3dacead__default.json"],
		contexts["test_1bde8445-ca19-4bda-a9d5-3543a800fc0f__context_values.json"],
		contexts["test_in_condition_json_array_format__should_match.jsonc"],
		contexts["test_in_condition_numeric_comma_separated__should_match.jsonc"],
		contexts["test_in_condition_array_matching_value__should_match.jsonc"],
	}
	var withoutDependencies, withDependencies []*engine_eval.EngineEvaluationContext
	for name, ec := range contexts {
		if strings.HasPrefix(name, "test_flag_dependency__") {
			withDependencies = append(withDependencies, ec)
		} else {
			withoutDependencies = append(withoutDependencies, ec)
		}
	}

	workloads := []struct {
		name     string
		contexts []*engine_eval.EngineEvaluationContext
	}{
		{"engine_test_data/upstream_benchmark", upstreamContexts},
		{"engine_test_data/without_flag_dependencies", withoutDependencies},
		{"engine_test_data/with_flag_dependencies", withDependencies},
		{"synthetic/without_flag_dependencies", []*engine_eval.EngineEvaluationContext{syntheticContext(0)}},
		{"synthetic/with_flag_dependencies", []*engine_eval.EngineEvaluationContext{syntheticContext(20)}},
	}

	for _, workload := range workloads {
		require.NotContains(b, workload.contexts, (*engine_eval.EngineEvaluationContext)(nil))
		b.Run(workload.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for _, ec := range workload.contexts {
					flagengine.GetEvaluationResult(ec)
				}
			}
		})
	}
}

// syntheticContext returns an identity context with 200 features and 100 segments, each segment
// overriding one feature on two trait conditions. The first dependentSegments segments instead
// condition on the flag their predecessor overrides, forming a chain of flag dependencies.
func syntheticContext(dependentSegments int) *engine_eval.EngineEvaluationContext {
	ec := &engine_eval.EngineEvaluationContext{
		Environment: engine_eval.EnvironmentContext{Key: "environment", Name: "Environment"},
		Identity: &engine_eval.IdentityContext{
			Identifier: "user",
			Key:        "environment_user",
			Traits:     map[string]any{"plan": "premium", "age": 42.0, "country": "PT", "beta": true, "email": "user@example.com"},
		},
		Features: map[string]engine_eval.FeatureContext{},
		Segments: map[string]engine_eval.SegmentContext{},
	}
	for i := range 200 {
		name := fmt.Sprintf("feature_%d", i)
		ec.Features[name] = engine_eval.FeatureContext{Key: fmt.Sprint(i), Name: name, Enabled: i%2 == 0, Value: "default"}
	}
	for i := range 100 {
		conditions := []engine_eval.Condition{
			{Operator: engine_eval.Equal, Property: "plan", Value: "premium"},
			{Operator: engine_eval.GreaterThan, Property: "age", Value: fmt.Sprint(i)},
		}
		if i > 0 && i <= dependentSegments {
			conditions = []engine_eval.Condition{
				{Operator: engine_eval.Equal, Property: fmt.Sprintf("$.flags.feature_%d.enabled", i-1), Value: "true"},
			}
		}
		key := fmt.Sprint(i)
		ec.Segments[key] = engine_eval.SegmentContext{
			Key:       key,
			Name:      "segment_" + key,
			Rules:     []engine_eval.SegmentRule{{Type: engine_eval.All, Conditions: conditions}},
			Overrides: []engine_eval.FeatureContext{{Key: key, Name: fmt.Sprintf("feature_%d", i), Enabled: true, Value: "override"}},
		}
	}
	return ec
}
