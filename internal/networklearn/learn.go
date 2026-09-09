package networklearn

import (
	"encoding/json"
	"io"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/pixel365/agbx/internal/networkpolicy"
)

type archive struct {
	Log archiveLog `json:"log"`
}

type archiveLog struct {
	Entries []archiveEntry `json:"entries"`
}

type archiveEntry struct {
	Request archiveRequest `json:"request"`
}

type archiveRequest struct {
	URL string `json:"url"`
}

func Destinations(reader io.Reader) ([]string, error) {
	var archive archive
	if err := json.NewDecoder(reader).Decode(&archive); err != nil {
		return nil, err
	}

	destinations := make(map[string]struct{})
	for _, entry := range archive.Log.Entries {
		host, ok := destinationHost(entry.Request.URL)
		if !ok {
			continue
		}
		destinations[host] = struct{}{}
	}

	result := make([]string, 0, len(destinations))
	for host := range destinations {
		result = append(result, host)
	}
	sort.Strings(result)

	return result, nil
}

func destinationHost(rawURL string) (string, bool) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(parsedURL.Hostname(), "."))
	if host == "" || net.ParseIP(host) != nil {
		return "", false
	}
	if err := (networkpolicy.Policy{
		Default: networkpolicy.DefaultDeny,
		Allow:   []string{host},
	}).Validate(); err != nil {
		return "", false
	}

	return host, true
}
