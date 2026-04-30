// SPDX-License-Identifier: EUPL-1.2

package app

func stringIndex(s, needle string) int {
	if needle == "" {
		return 0
	}
	if len(needle) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(needle); i++ {
		if s[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func stringLastIndex(s, needle string) int {
	if needle == "" {
		return len(s)
	}
	if len(needle) > len(s) {
		return -1
	}
	for i := len(s) - len(needle); i >= 0; i-- {
		if s[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func stringIndexAny(s, chars string) int {
	for i, r := range s {
		for _, c := range chars {
			if r == c {
				return i
			}
		}
	}
	return -1
}
