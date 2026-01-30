package parser

import (
	"net"
	"strings"
)

func ExtractIP(line string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", false
	}

	ip := fields[0]
	if net.ParseIP(ip) == nil {
		return "", false
	}
	return ip, true
}
