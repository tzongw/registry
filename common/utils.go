package common

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

func MergeMap(first map[string]string, second map[string]string) map[string]string {
	m := make(map[string]string, len(first)+len(second))
	for k, v := range first {
		m[k] = v
	}
	for k, v := range second {
		m[k] = v
	}
	return m
}
