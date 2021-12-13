package metadnsq

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

type testCase struct {
	input       string
	shouldErr   bool
	expectedErr string
}

func (t testCase) String() string {
	return fmt.Sprintf("{%T input=%q shouldErr=%v expectedErr=%q}",
		t, t.input, t.shouldErr, t.expectedErr)
}

// Return true if test passed, false otherwise
func (t *testCase) Pass(err error) bool {
	// Empty expected error isn't allow
	if t.shouldErr == (len(t.expectedErr) == 0) {
		panic(fmt.Sprintf("Bad test case %v", t))
	}

	pass := true
	if t.shouldErr == (err != nil) {
		if err != nil {
			s := strings.ToLower(err.Error())
			t := strings.ToLower(t.expectedErr)
			if !strings.Contains(s, t) {
				pass = false
			}
		}
	} else {
		pass = false
	}
	return pass
}

func TestSetupTo(t *testing.T) {
	tests := []testCase{
		// Negative
		{"dnssrc", true, `missing mandatory property: "to"`},
		{"dnssrc .", true, `missing mandatory property: "to"`},
		// {"dnssrc lips.conf { }", true, `missing mandatory property: "to"`},
		{"dnssrc . { to }", true, "not an IP address:"},
		{"dnssrc . { to . }", true, "not an IP address:"},
		{"dnssrc . { to foo }", true, "not an IP address:"},
		{"dnssrc . { to foobar.net }", true, "not an IP address:"},
		{"dnssrc . { to foobar.net. }", true, "not an IP address:"},
		{"dnssrc . { to 1.2.3 }", true, "not an IP address:"},
		{"dnssrc . { to 1.2.3.4 }", true, "not an IP address:"},
		{"dnssrc . { to \n }", true, "Wrong argument count or unexpected line ending after"},
		{"dnssrc . { to . \n }", true, "not an IP address:"},
		{"dnssrc . { to / \n }", true, "not an IP address:"},
		{"dnssrc . { to 1.2.3. \n }", true, "not an IP address:"},
		{"dnssrc . { to foobar://1.1.1.1 \n }", true, "not an IP address:"},
		// Positive
		{"dnssrc . { to 1.2.3.4 \n }", false, ""},
		{"dnssrc . { to 1.2.3.4 / . \n }", true, "not an IP address:"},
		{"dnssrc . { to dns://8.8.4.4 \n }", false, ""},
		{"dnssrc . { to dns://192.168.144.10:5353 \n }", false, ""},
		{"dnssrc . { to tls://172.16.10.1 \n }", false, ""},
		{"dnssrc . { to tls://172.16.10.1:1234 \n }", false, ""},
		{"dnssrc . { to 10.1.2.3 dns://192.168.144.100 tls://172.16.10.1:1234 \n }", false, ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		_, err := newReloadableUpstream(c)
		if !test.Pass(err) {
			t.Errorf("Test#%v failed  %v vs err: %v", i, test, err)
		}
	}
}

func TestParse(t *testing.T) {
	c := caddy.NewTestController("dns", `dnssrc lips.conf {
        path_reload 3s
        max_fails 0
        to 114.114.114.114 223.5.5.5 udp://119.29.29.29
        policy round_robin
    }`)
	item, err := newReloadableUpstream(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(item)
}
