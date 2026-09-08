package networkpolicy

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

const (
	DefaultAllow         = "allow"
	DefaultDeny          = "deny"
	invalidHostnameError = "is not a valid hostname"
)

type Policy struct {
	Default string   `yaml:"default"`
	Allow   []string `yaml:"allow,omitempty"`
	Deny    []string `yaml:"deny,omitempty"`
}

type Additions struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

func (p Policy) Validate() error {
	if p.Default != DefaultAllow && p.Default != DefaultDeny {
		return fmt.Errorf("default must be %q or %q", DefaultAllow, DefaultDeny)
	}
	if err := validatePatterns("allow", p.Allow); err != nil {
		return err
	}
	if err := validatePatterns("deny", p.Deny); err != nil {
		return err
	}

	return nil
}

func (a Additions) Validate() error {
	if err := validatePatterns("allow", a.Allow); err != nil {
		return err
	}
	if err := validatePatterns("deny", a.Deny); err != nil {
		return err
	}

	return nil
}

func (a Additions) IsEmpty() bool {
	return len(a.Allow) == 0 && len(a.Deny) == 0
}

func (p Policy) With(additions Additions) Policy {
	result := Policy{Default: p.Default}
	result.Allow = append(result.Allow, p.Allow...)
	result.Allow = append(result.Allow, additions.Allow...)
	result.Deny = append(result.Deny, p.Deny...)
	result.Deny = append(result.Deny, additions.Deny...)

	return result
}

func (p Policy) Allows(host string, port int) bool {
	if port != 80 && port != 443 {
		return false
	}

	host = strings.Trim(strings.ToLower(strings.TrimSuffix(host, ".")), "[]")
	if net.ParseIP(host) != nil {
		return false
	}
	if matchesAny(p.Deny, host) {
		return false
	}
	if matchesAny(p.Allow, host) {
		return true
	}

	return p.Default == DefaultAllow
}

func validatePatterns(kind string, patterns []string) error {
	knownPatterns := make(map[string]struct{}, len(patterns))
	for index, pattern := range patterns {
		if err := validatePattern(pattern); err != nil {
			return fmt.Errorf("%s host %d: %w", kind, index+1, err)
		}
		key := strings.ToLower(pattern)
		if _, found := knownPatterns[key]; found {
			return fmt.Errorf("%s host %q is duplicated", kind, pattern)
		}
		knownPatterns[key] = struct{}{}
	}

	return nil
}

func validatePattern(pattern string) error {
	if pattern == "" {
		return errors.New("is required")
	}
	if strings.TrimSpace(pattern) != pattern {
		return errors.New("must not contain surrounding whitespace")
	}

	host, err := patternHostname(pattern)
	if err != nil {
		return err
	}

	return validateHostname(host)
}

func patternHostname(pattern string) (string, error) {
	host := strings.TrimPrefix(pattern, "*.")
	if strings.HasPrefix(host, "*.") || strings.Contains(host, "*") {
		return "", errors.New("must be an exact hostname or start with \"*.\"")
	}

	return host, nil
}

func validateHostname(host string) error {
	if net.ParseIP(host) != nil {
		return errors.New("must not be an IP address")
	}
	if len(host) > 253 || strings.HasSuffix(host, ".") {
		return errors.New(invalidHostnameError)
	}

	for label := range strings.SplitSeq(host, ".") {
		if err := validateHostnameLabel(label); err != nil {
			return err
		}
	}

	return nil
}

func validateHostnameLabel(label string) error {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return errors.New(invalidHostnameError)
	}
	for _, character := range label {
		if isHostnameCharacter(character) {
			continue
		}

		return errors.New(invalidHostnameError)
	}

	return nil
}

func isHostnameCharacter(character rune) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' ||
		character == '-'
}

func matchesAny(patterns []string, host string) bool {
	for _, pattern := range patterns {
		if matches(pattern, host) {
			return true
		}
	}

	return false
}

func matches(pattern string, host string) bool {
	pattern = strings.ToLower(pattern)
	if !strings.HasPrefix(pattern, "*.") {
		return host == pattern
	}

	return strings.HasSuffix(host, pattern[1:]) && host != pattern[2:]
}
