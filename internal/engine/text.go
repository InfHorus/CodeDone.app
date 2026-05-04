package engine

import "strings"

func ensureBullet(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return "- "
	}
	if strings.HasPrefix(line, "- ") {
		return line
	}
	return "- " + line
}

func ensureBulletLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, ensureBullet(line))
	}
	return out
}
