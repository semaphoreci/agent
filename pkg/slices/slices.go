package slices

func Contains(slice []string, item string) bool {
	for _, x := range slice {
		if x == item {
			return true
		}
	}

	return false
}
