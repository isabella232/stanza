package helper

import (
	"testing"

	"github.com/observiq/carbon/entry"
	"github.com/stretchr/testify/require"
)

func MockHostLabelerConfig(includeIP, includeHostname bool, ip, hostname string) HostLabelerConfig {
	return HostLabelerConfig{
		IncludeIP:       includeIP,
		IncludeHostname: includeHostname,
		getIP:           func() (string, error) { return ip, nil },
		getHostname:     func() (string, error) { return hostname, nil },
	}
}

func TestHostLabeler(t *testing.T) {
	cases := []struct {
		name           string
		config         HostLabelerConfig
		expectedLabels map[string]string
	}{
		{
			"HostnameAndIP",
			MockHostLabelerConfig(true, true, "ip", "hostname"),
			map[string]string{
				"hostname": "hostname",
				"ip":       "ip",
			},
		},
		{
			"HostnameNoIP",
			MockHostLabelerConfig(false, true, "ip", "hostname"),
			map[string]string{
				"hostname": "hostname",
			},
		},
		{
			"IPNoHostname",
			MockHostLabelerConfig(true, false, "ip", "hostname"),
			map[string]string{
				"ip": "ip",
			},
		},
		{
			"NoHostnameNoIP",
			MockHostLabelerConfig(false, false, "", "test"),
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			labeler, err := tc.config.Build()
			require.NoError(t, err)

			e := entry.New()
			labeler.Label(e)
			require.Equal(t, tc.expectedLabels, e.Labels)
		})
	}
}