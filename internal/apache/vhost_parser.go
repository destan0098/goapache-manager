package apache

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

type VHost struct {
	Domain  string `json:"domain"`
	Port    int    `json:"port"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Default bool   `json:"default"`
}

var (
	nameVHostRegex = regexp.MustCompile(
		`^\s*port\s+(\d+)\s+namevhost\s+(\S+)\s+\((.+):(\d+)\)\s*$`,
	)

	defaultRegex = regexp.MustCompile(
		`^\s*default server\s+(\S+)\s+\((.+):(\d+)\)\s*$`,
	)
)

func ParseVHosts(output string) []VHost {
	var result []VHost
	defaults := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()

		if matches := defaultRegex.FindStringSubmatch(line); matches != nil {
			port := 80

			// پورت از خطوط namevhost مشخص می‌شود.
			// فعلاً دامنه و فایل پیش‌فرض را ثبت می‌کنیم.
			key := matches[1] + "|" + matches[2]
			defaults[key] = true
			_ = port
		}
	}

	scanner = bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		matches := nameVHostRegex.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}

		port, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		line, err := strconv.Atoi(matches[4])
		if err != nil {
			continue
		}

		vhost := VHost{
			Domain: matches[2],
			Port:   port,
			File:   matches[3],
			Line:   line,
		}

		vhost.Default = defaults[vhost.Domain+"|"+vhost.File]

		result = append(result, vhost)
	}

	return result
}
