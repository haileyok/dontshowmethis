package main

import (
	"net/url"
	"testing"
)

func TestIsPolDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://www.Axios.com/stupid-story", true},
		{"https://axios.com/stupid-story", true},
		{"https://haileyok.com/posts/3kq5ffbmd2223", false},
	}

	for _, test := range tests {
		u, err := url.Parse(test.url)
		if err != nil {
			t.Fatalf("Failed to parse URL %s: %v", test.url, err)
		}
		result := isPolDomain(u)
		if result != test.expected {
			t.Errorf("isPolDomain(%s) = %v; expected %v", test.url, result, test.expected)
		}
	}
}
