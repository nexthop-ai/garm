// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package metrics

import "testing"

func TestNormalizeRequestedLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected string
	}{
		{
			name:     "nil labels",
			labels:   nil,
			expected: "",
		},
		{
			name:     "empty labels",
			labels:   []string{},
			expected: "",
		},
		{
			name:     "labels are sorted",
			labels:   []string{"x64", "self-hosted", "linux"},
			expected: "linux,self-hosted,x64",
		},
		{
			name:     "permutations collapse to the same value",
			labels:   []string{"linux", "x64", "self-hosted"},
			expected: "linux,self-hosted,x64",
		},
		{
			name:     "blank and duplicate labels are dropped",
			labels:   []string{"linux", "  ", "linux", ""},
			expected: "linux",
		},
		{
			name:     "surrounding whitespace is trimmed",
			labels:   []string{" self-hosted ", "linux"},
			expected: "linux,self-hosted",
		},
		{
			// GitHub matches runner labels case-insensitively.
			name:     "labels are lowercased",
			labels:   []string{"Linux", "Self-Hosted", "X64"},
			expected: "linux,self-hosted,x64",
		},
		{
			name:     "case variants of one label collapse",
			labels:   []string{"linux", "Linux", "LINUX"},
			expected: "linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeRequestedLabels(tt.labels); got != tt.expected {
				t.Errorf("NormalizeRequestedLabels() = %q, want %q", got, tt.expected)
			}
		})
	}
}
