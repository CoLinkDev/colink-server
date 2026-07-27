package ws

import "strconv"

type Semver struct {
	Major int
	Minor int
	Patch int
}

func ParseSemver(value string) (Semver, bool) {
	var version Semver
	parts := splitSemver(value)
	if len(parts) != 3 {
		return version, false
	}

	values := [3]*int{&version.Major, &version.Minor, &version.Patch}
	for index, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return Semver{}, false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return Semver{}, false
			}
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return Semver{}, false
		}
		*values[index] = parsed
	}

	return version, true
}

func splitSemver(value string) []string {
	parts := make([]string, 0, 3)
	start := 0
	for index := 0; index < len(value); index++ {
		if value[index] != '.' {
			continue
		}
		parts = append(parts, value[start:index])
		start = index + 1
	}
	parts = append(parts, value[start:])
	return parts
}
