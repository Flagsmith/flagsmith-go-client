package segments

import "strconv"

func match(c ConditionOperator, s1, s2 string) bool {
	b1, e1 := strconv.ParseBool(s1)
	b2, e2 := strconv.ParseBool(s2)
	if e1 == nil && e2 == nil {
		return matchBool(c, b1, b2)
	}

	i1, e1 := strconv.ParseInt(s1, 10, 64)
	i2, e2 := strconv.ParseInt(s2, 10, 64)
	if e1 == nil && e2 == nil {
		return matchInt(c, i1, i2)
	}

	f1, e1 := strconv.ParseFloat(s1, 64)
	f2, e2 := strconv.ParseFloat(s2, 64)
	if e1 == nil && e2 == nil {
		return matchFloat(c, f1, f2)
	}

	return matchString(c, s1, s2)
}

func matchBool(c ConditionOperator, v1, v2 bool) bool {
	var i1, i2 int64
	if v1 {
		i1 = 1
	}
	if v2 {
		i2 = 1
	}
	return matchInt(c, i1, i2)
}

func matchInt(c ConditionOperator, v1, v2 int64) bool {
	switch c {
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func matchFloat(c ConditionOperator, v1, v2 float64) bool {
	switch c {
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}

func matchString(c ConditionOperator, v1, v2 string) bool {
	switch c {
	case Equal:
		return v1 == v2
	case GreaterThan:
		return v1 > v2
	case LessThan:
		return v1 < v2
	case LessThanInclusive:
		return v1 <= v2
	case GreaterThanInclusive:
		return v1 >= v2
	case NotEqual:
		return v1 != v2
	}
	return v1 == v2
}
