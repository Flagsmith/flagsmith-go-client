package utils

func All(args []bool) bool {
	for _, a := range args {
		if a == false {
			return false
		}
	}
	return true
}

func Any(args []bool) bool {
	for _, a := range args {
		if a == true {
			return true
		}
	}
	return false
}
