package common

func FindIndex(limit int, predicate func(i int) bool) int {
	for i := 0; i < limit; i++ {
		if predicate(i) {
			return i
		}
	}
	return -1
}

func MergeMap(first map[string]string, second map[string]string) map[string]string {
	m := make(map[string]string)
	for k, v := range first {
		m[k] = v
	}
	for k, v := range second {
		m[k] = v
	}
	return m
}