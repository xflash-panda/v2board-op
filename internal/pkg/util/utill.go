package util

import (
	"fmt"
	"regexp"
	"strings"
)

func IsValidDomain(domain string) bool {
	regex := regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	return regex.MatchString(domain)
}

func GetRootDomain(domain string) (string, error) {
	parts := strings.Split(domain, ".")
	numParts := len(parts)

	// Check if the domain has at least two parts
	if numParts < 2 {
		return "", fmt.Errorf("invalid domain")
	}

	// Extract the root domain
	rootDomain := parts[numParts-2] + "." + parts[numParts-1]
	return rootDomain, nil
}
