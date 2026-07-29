// Copyright 2025 Cloudbase Solutions SRL
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

package params

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPool_HasRequiredLabels(t *testing.T) {
	tests := []struct {
		name           string
		poolTags       []Tag
		requiredLabels []string
		expected       bool
	}{
		{
			name:           "empty pool tags and empty required labels",
			poolTags:       []Tag{},
			requiredLabels: []string{},
			expected:       true,
		},
		{
			name:           "empty pool tags with required labels",
			poolTags:       []Tag{},
			requiredLabels: []string{"label1"},
			expected:       false,
		},
		{
			name: "pool has all required labels - exact match",
			poolTags: []Tag{
				{Name: "label1"},
				{Name: "label2"},
				{Name: "label3"},
			},
			requiredLabels: []string{"label1", "label2"},
			expected:       true,
		},
		{
			name: "pool missing one required label",
			poolTags: []Tag{
				{Name: "label1"},
				{Name: "label3"},
			},
			requiredLabels: []string{"label1", "label2"},
			expected:       false,
		},
		{
			name: "pool has more labels than required",
			poolTags: []Tag{
				{Name: "label1"},
				{Name: "label2"},
				{Name: "label3"},
				{Name: "label4"},
			},
			requiredLabels: []string{"label1", "label2"},
			expected:       true,
		},
		{
			name: "case insensitive matching - lowercase required",
			poolTags: []Tag{
				{Name: "Label1"},
				{Name: "LABEL2"},
			},
			requiredLabels: []string{"label1", "label2"},
			expected:       true,
		},
		{
			name: "case insensitive matching - uppercase required",
			poolTags: []Tag{
				{Name: "label1"},
				{Name: "label2"},
			},
			requiredLabels: []string{"LABEL1", "LABEL2"},
			expected:       true,
		},
		{
			name: "case insensitive matching - mixed case",
			poolTags: []Tag{
				{Name: "LaBel1"},
				{Name: "lAbEl2"},
			},
			requiredLabels: []string{"lAbEl1", "LaBel2"},
			expected:       true,
		},
		{
			name: "more required labels than pool tags",
			poolTags: []Tag{
				{Name: "label1"},
			},
			requiredLabels: []string{"label1", "label2", "label3"},
			expected:       false,
		},
		{
			name: "exact match with single label",
			poolTags: []Tag{
				{Name: "ubuntu"},
			},
			requiredLabels: []string{"ubuntu"},
			expected:       true,
		},
		{
			name: "no match with single label",
			poolTags: []Tag{
				{Name: "ubuntu"},
			},
			requiredLabels: []string{"windows"},
			expected:       false,
		},
		{
			name: "duplicate required labels",
			poolTags: []Tag{
				{Name: "label1"},
				{Name: "label2"},
			},
			requiredLabels: []string{"label1", "label1"},
			expected:       true,
		},
		{
			name: "special characters in labels",
			poolTags: []Tag{
				{Name: "self-hosted"},
				{Name: "linux-x64"},
			},
			requiredLabels: []string{"self-hosted", "linux-x64"},
			expected:       true,
		},
		{
			name: "similar but not exact labels",
			poolTags: []Tag{
				{Name: "label1"},
				{Name: "label12"},
			},
			requiredLabels: []string{"label1", "label2"},
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &Pool{
				Tags: tt.poolTags,
			}
			result := pool.HasRequiredLabels(tt.requiredLabels)
			if result != tt.expected {
				t.Errorf("HasRequiredLabels() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestValidateGARMAgentVersion(t *testing.T) {
	valid := []string{
		"",
		GARMAgentLatestVersion,
		"v1.2.3",
		"1.2.3",
		"v0.1.0",
		"v1.2.3-rc1",
		"1.2.3-beta.1",
		"v1.2.3+build.5",
	}
	for _, version := range valid {
		if err := ValidateGARMAgentVersion(version); err != nil {
			t.Errorf("expected version %q to be valid, got: %v", version, err)
		}
	}

	invalid := []string{
		"Latest", // the keyword is case sensitive
		"v1",
		"v1.2", // x/mod shorthands are not valid semver
		"1.2",
		"v1.2.3.4",
		"not-a-version",
		"v1.2.x",
		"v 1.2.3",
		"1.2.3 ",
	}
	for _, version := range invalid {
		if err := ValidateGARMAgentVersion(version); err == nil {
			t.Errorf("expected version %q to be rejected", version)
		}
	}
}

func TestUpdateControllerParamsValidateAgentVersion(t *testing.T) {
	version := "v1.2.3"
	if err := (UpdateControllerParams{GARMAgentVersion: &version}).Validate(); err != nil {
		t.Errorf("expected %q to validate, got: %v", version, err)
	}

	// Surrounding whitespace is trimmed before validation.
	version = "  latest  "
	if err := (UpdateControllerParams{GARMAgentVersion: &version}).Validate(); err != nil {
		t.Errorf("expected %q to validate, got: %v", version, err)
	}

	version = "bogus"
	if err := (UpdateControllerParams{GARMAgentVersion: &version}).Validate(); err == nil {
		t.Errorf("expected %q to be rejected", version)
	}
}

// TestWorkflowJobUnmarshalTimestamps ensures the webhook payload keeps the
// forge supplied timestamps we need to compute queue latency. GitHub sends
// created_at on workflow_job events; without it we would have to fall back on
// GARM's local receive time, which lags the real queue time.
func TestWorkflowJobUnmarshalTimestamps(t *testing.T) {
	payload := []byte(`{
		"action": "in_progress",
		"workflow_job": {
			"id": 1234,
			"run_id": 5678,
			"status": "in_progress",
			"created_at": "2026-07-29T10:00:00Z",
			"started_at": "2026-07-29T10:05:00Z",
			"completed_at": null,
			"labels": ["self-hosted", "linux"],
			"runner_name": "garm-runner-1"
		}
	}`)

	var job WorkflowJob
	if err := json.Unmarshal(payload, &job); err != nil {
		t.Fatalf("unmarshalling workflow job: %v", err)
	}

	wantCreated := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	wantStarted := time.Date(2026, 7, 29, 10, 5, 0, 0, time.UTC)

	if !job.WorkflowJob.CreatedAt.Equal(wantCreated) {
		t.Errorf("created_at = %v, want %v", job.WorkflowJob.CreatedAt, wantCreated)
	}
	if !job.WorkflowJob.StartedAt.Equal(wantStarted) {
		t.Errorf("started_at = %v, want %v", job.WorkflowJob.StartedAt, wantStarted)
	}
	if got := job.WorkflowJob.StartedAt.Sub(job.WorkflowJob.CreatedAt); got != 5*time.Minute {
		t.Errorf("queue duration = %v, want %v", got, 5*time.Minute)
	}
}

// TestWorkflowJobUnmarshalMissingCreatedAt documents that an absent created_at
// leaves a zero timestamp, which callers must treat as "unknown".
func TestWorkflowJobUnmarshalMissingCreatedAt(t *testing.T) {
	var job WorkflowJob
	if err := json.Unmarshal([]byte(`{"action":"in_progress","workflow_job":{"id":1}}`), &job); err != nil {
		t.Fatalf("unmarshalling workflow job: %v", err)
	}
	if !job.WorkflowJob.CreatedAt.IsZero() {
		t.Errorf("created_at = %v, want zero value", job.WorkflowJob.CreatedAt)
	}
}
