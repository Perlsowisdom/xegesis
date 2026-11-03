package main

import "testing"

func TestScreenshotArg(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"--screenshot", "a.png"}, "a.png"},
		{[]string{"-x", "--screenshot=b c.png"}, "b c.png"},
		{[]string{"--screenshot"}, ""},
	} {
		if got := screenshotArg(c.args); got != c.want {
			t.Errorf("screenshotArg(%q) = %q, want %q", c.args, got, c.want)
		}
	}
}
