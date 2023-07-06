package segments

type RuleType string
type ConditionOperator string

const (
	All  RuleType = "ALL"
	Any  RuleType = "ANY"
	None RuleType = "NONE"

	Equal                ConditionOperator = "EQUAL"
	GreaterThan          ConditionOperator = "GREATER_THAN"
	LessThan             ConditionOperator = "LESS_THAN"
	LessThanInclusive    ConditionOperator = "LESS_THAN_INCLUSIVE"
	Contains             ConditionOperator = "CONTAINS"
	GreaterThanInclusive ConditionOperator = "GREATER_THAN_INCLUSIVE"
	NotContains          ConditionOperator = "NOT_CONTAINS"
	NotEqual             ConditionOperator = "NOT EQUAL"
	Regex                ConditionOperator = "REGEX"
	PercentageSplit      ConditionOperator = "PERCENTAGE_SPLIT"
	IsSet                ConditionOperator = "IS_SET"
	IsNotSet             ConditionOperator = "IS_NOT_SET"
	Modulo               ConditionOperator = "MODULO"
	In                   ConditionOperator = "IN"
)
