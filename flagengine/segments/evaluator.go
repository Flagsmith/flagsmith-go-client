package segments

import (
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities/traits"
)

func EvaluateIdentityInSegment(
	identity *identities.IdentityModel,
	segment *SegmentModel,
	overrideTraits ...*traits.TraitModel,
) bool {
	return true
}
