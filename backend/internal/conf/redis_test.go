package conf

import (
	"reflect"
	"testing"
)

func TestRedisSeedAddrsAndCluster(t *testing.T) {
	cases := []struct {
		name    string
		redis   Redis
		want    []string
		cluster bool
	}{
		{"single addr", Redis{Addr: "localhost:6379"}, []string{"localhost:6379"}, false},
		// The reported production case: a comma list in the single addr env/field.
		{"comma list in single addr",
			Redis{Addr: "10.1.114.1:7000,10.1.114.2:7000,10.1.114.3:7000"},
			[]string{"10.1.114.1:7000", "10.1.114.2:7000", "10.1.114.3:7000"}, true},
		{"addrs list", Redis{Addrs: []string{"a:7000", "b:7000"}}, []string{"a:7000", "b:7000"}, true},
		{"addrs with a comma entry", Redis{Addrs: []string{"a:7000, b:7000", "c:7000"}},
			[]string{"a:7000", "b:7000", "c:7000"}, true},
		{"single seed forced cluster", Redis{Addr: "vip:6379", Cluster: true}, []string{"vip:6379"}, true},
		{"sentinel not cluster", Redis{Addrs: []string{"s1:26379", "s2:26379"}, MasterName: "mymaster"},
			[]string{"s1:26379", "s2:26379"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.redis.SeedAddrs(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SeedAddrs() = %v, want %v", got, tc.want)
			}
			if got := tc.redis.IsCluster(); got != tc.cluster {
				t.Errorf("IsCluster() = %v, want %v", got, tc.cluster)
			}
		})
	}
}
