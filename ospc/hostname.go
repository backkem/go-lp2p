package ospc

import (
	"fmt"
	"regexp"
	"strings"
)

// encodeHostnamePart encodes a string for use in hostname by replacing
// non-alphanumeric characters with hyphens per OpenScreen spec
func encodeHostnamePart(input string) string {
	// Replace non-alphanumeric characters with hyphens
	re := regexp.MustCompile(`[^a-zA-Z0-9]`)
	return re.ReplaceAllString(input, "-")
}

// buildAgentHostname constructs an OpenScreen-compliant agent hostname
// Format: serialNumber.encodedInstanceName.encodedDomain
func buildAgentHostname(serialNumberBase64, instanceName, domain string) string {
	encodedInstance := encodeHostnamePart(instanceName)
	encodedDomain := encodeHostnamePart(domain)
	return fmt.Sprintf("%s.%s.%s", serialNumberBase64, encodedInstance, encodedDomain)
}

// parseAgentHostname parses an agent hostname back into its components
func parseAgentHostname(hostname string) (serialNumber, instanceName, domain string, err error) {
	parts := strings.Split(hostname, ".")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid hostname format: expected at least 3 parts, got %d", len(parts))
	}
	
	serialNumber = parts[0]
	instanceName = parts[1]
	domain = strings.Join(parts[2:], ".")
	
	return serialNumber, instanceName, domain, nil
}