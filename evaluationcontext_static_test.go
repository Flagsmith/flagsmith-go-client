package flagsmith_test

import (
    "testing"

    flagsmith "github.com/Flagsmith/flagsmith-go-client/v4"
    "github.com/Flagsmith/flagsmith-go-client/v4/fixtures"
    "github.com/stretchr/testify/assert"
)

func TestMapEnvironmentDocumentToEvaluationContext(t *testing.T) {
    // Given
    envJSON := []byte(fixtures.EnvironmentJson)

    // When
    ctx, err := flagsmith.MapEnvironmentDocumentToEvaluationContext(envJSON)

    // Then
    assert.NoError(t, err)

    // Environment
    assert.Equal(t, fixtures.ClientAPIKey, ctx.Environment.APIKey)
    assert.Equal(t, "Test project", ctx.Environment.Name)

    // Features
    if assert.Len(t, ctx.Features, 1) {
        f, ok := ctx.Features[fixtures.Feature1Name]
        if assert.True(t, ok, "feature_1 should exist") {
            assert.True(t, f.Enabled)
            assert.Equal(t, fixtures.Feature1Name, f.FeatureKey)
            assert.Equal(t, fixtures.Feature1Name, f.Name)
            assert.Equal(t, fixtures.Feature1Value, f.Value)
            // key should come from featurestate_uuid when django_id is absent
            assert.Equal(t, "40eb539d-3713-4720-bbd4-829dbef10d51", f.Key)
            assert.Nil(t, f.Priority)
            assert.Len(t, f.Variants, 0)
        }
    }

    // Segments
    if assert.Len(t, ctx.Segments, 1) {
        s, ok := ctx.Segments["Test Segment"]
        if assert.True(t, ok, "segment should exist by name") {
            assert.Equal(t, "1", s.Key)
            // At least one rule exists
            assert.GreaterOrEqual(t, len(s.Rules), 1)

            // Find a condition with property "foo" and value "bar"
            found := false
            var walkRules func(r flagsmith.SegmentRule)
            walkRules = func(r flagsmith.SegmentRule) {
                for _, c := range r.Conditions {
                    if c.Property == "foo" && c.Value != nil && c.Value.String != nil && *c.Value.String == "bar" {
                        found = true
                        return
                    }
                }
                for _, nr := range r.Rules {
                    if found {
                        return
                    }
                    walkRules(nr)
                }
            }
            for _, r := range s.Rules {
                if found {
                    break
                }
                walkRules(r)
            }
            assert.True(t, found, "expected to find condition property_ == foo with value bar")
        }
    }
}


