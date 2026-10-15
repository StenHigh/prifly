//go:build darwin || linux

package local

import "testing"

func TestDispatcherEvidenceDistinguishesOldLifetimeFromLiveProcess(t *testing.T) {
	t.Parallel()
	current, err := CurrentDispatcher()
	if err != nil {
		t.Fatal(err)
	}
	if DispatcherGone(current) {
		t.Fatal("live dispatcher was treated as terminated")
	}
	cases := []struct {
		name     string
		identity DispatcherIdentity
		gone     bool
	}{
		{"new boot on the same host", func() DispatcherIdentity {
			id := current
			id.BootID = "00000000-0000-0000-0000-000000000000"
			return id
		}(), true},
		{"PID now names a different lifetime", func() DispatcherIdentity { id := current; id.StartID = "previous-native-start-identity"; return id }(), true},
		{"UID change alone is not lifetime termination", func() DispatcherIdentity { id := current; id.UID++; return id }(), false},
		{"another host", func() DispatcherIdentity { id := current; id.Hostname += "-different"; return id }(), false},
		{"missing boot evidence", func() DispatcherIdentity { id := current; id.BootID = ""; return id }(), false},
		{"missing start evidence", func() DispatcherIdentity { id := current; id.StartID = ""; return id }(), false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if gone := DispatcherGone(test.identity); gone != test.gone {
				t.Fatalf("former dispatcher gone=%v, want %v", gone, test.gone)
			}
		})
	}
}
