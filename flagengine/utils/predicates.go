package utils

func All(args []bool) bool {
	for _, a := range args {
		if !a {
			return false
		}
	}
	return true
}

func Any(args []bool) bool {
	for _, a := range args {
		if a {
			return true
		}
	}
	return false
}

func None(args []bool) bool {
	return !Any(args)
}
