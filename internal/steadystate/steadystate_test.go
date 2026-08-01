package steadystate

import "testing"

func TestViolationString(t *testing.T) {
	cases := []struct {
		name string
		in   Violation
		want string
	}{
		{
			name: "attack target vanished",
			in:   Violation{Check: "redis key secret:flag in target (app=redis)", Field: "dataExposed", Want: true, Got: false},
			want: "redis key secret:flag in target (app=redis): dataExposed is false, want true",
		},
		{
			name: "control was already down",
			in:   Violation{Check: "redis exfil from attacker", Field: "reachable", Want: false, Got: true},
			want: "redis exfil from attacker: reachable is true, want false",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.String(); got != c.want {
				t.Errorf("String() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		name string
		in   []Violation
		want string
	}{
		{
			name: "steady state intact",
			in:   nil,
			want: "",
		},
		{
			name: "single violation",
			in:   []Violation{{Check: "a", Field: "reachable", Want: true, Got: false}},
			want: "a: reachable is false, want true",
		},
		{
			name: "several violations are joined",
			in: []Violation{
				{Check: "a", Field: "reachable", Want: true, Got: false},
				{Check: "b", Field: "dataExposed", Want: false, Got: true},
			},
			want: "a: reachable is false, want true; b: dataExposed is true, want false",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Format(c.in); got != c.want {
				t.Errorf("Format() = %q, want %q", got, c.want)
			}
		})
	}
}
