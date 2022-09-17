package base

import "net"

func HostPort(hp string) (host, port string, err error) {
	if host, port, err = net.SplitHostPort(hp); err != nil {
		return
	}
	host, err = Extract(host)
	return
}

func FindIndex[T comparable](elems []T, v T) int {
	for index, elem := range elems {
		if v == elem {
			return index
		}
	}
	return -1
}

func Contains[T comparable](elems []T, v T) bool {
	return FindIndex(elems, v) >= 0
}

func MergeMap[K comparable, V any](first map[K]V, second map[K]V) map[K]V {
	m := make(map[K]V, len(first)+len(second))
	for k, v := range first {
		m[k] = v
	}
	for k, v := range second {
		m[k] = v
	}
	return m
}
