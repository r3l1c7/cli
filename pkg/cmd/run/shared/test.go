package shared

import (
	"fmt"
	"time"

	workflowShared "github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
)

var TestRunStartTime, _ = time.Parse("2006-01-02 15:04:05", "2021-02-23 04:51:00")

func TestRun(id int64, s Status, c Conclusion) Run {
	return TestRunWithCommit(id, s, c, "cool commit")
}

func TestRunWithCommit(id int64, s Status, c Conclusion, commit string) Run {
	return TestRunWithWorkflowAndCommit(123, id, s, c, commit)
}

func TestRunWithOrgRequiredWorkflow(id int64, s Status, c Conclusion, commit string) Run {
	return TestRunWithWorkflowAndCommit(456, id, s, c, commit)
}

func TestRunWithWorkflowAndCommit(workflowId, runId int64, s Status, c Conclusion, commit string) Run {
	return Run{
		WorkflowID: workflowId,
		ID:         runId,
		CreatedAt:  TestRunStartTime,
		UpdatedAt:  TestRunStartTime.Add(time.Minute*4 + time.Second*34),
		Status:     s,
		Conclusion: c,
		Event:      "push",
		HeadBranch: "trunk",
		JobsURL:    fmt.Sprintf("https://api.github.com/runs/%d/jobs", runId),
		HeadCommit: Commit{
			Message: commit,
		},
		HeadSha: "1234567890",
		URL:     fmt.Sprintf("https://github.com/runs/%d", runId),
		HeadRepository: Repo{
			Owner: struct{ Login string }{Login: "OWNER"},
			Name:  "REPO",
		},
	}
}

var SuccessfulRun Run = TestRun(3, Completed, Success)
var FailedRun Run = TestRun(1234, Completed, Failure)

var TestRuns []Run = []Run{
	TestRun(1, Completed, TimedOut),
	TestRun(2, InProgress, ""),
	SuccessfulRun,
	TestRun(4, Completed, Cancelled),
	FailedRun,
	TestRun(6, Completed, Neutral),
	TestRun(7, Completed, Skipped),
	TestRun(8, Requested, ""),
	TestRun(9, Queued, ""),
	TestRun(10, Completed, Stale),
}

var TestRunsWithOrgRequiredWorkflows []Run = []Run{
	TestRunWithOrgRequiredWorkflow(1, Completed, TimedOut, "cool commit"),
	TestRunWithOrgRequiredWorkflow(2, InProgress, "", "cool commit"),
	TestRunWithOrgRequiredWorkflow(3, Completed, Success, "cool commit"),
	TestRunWithOrgRequiredWorkflow(4, Completed, Cancelled, "cool commit"),
	TestRun(5, Completed, Failure),
	TestRun(6, Completed, Neutral),
	TestRun(7, Completed, Skipped),
	TestRun(8, Requested, ""),
	TestRun(9, Queued, ""),
}

var WorkflowRuns []Run = []Run{
	TestRun(2, InProgress, ""),
	SuccessfulRun,
	FailedRun,
}

var SuccessfulJob Job = Job{
	ID:          10,
	Status:      Completed,
	Conclusion:  Success,
	Name:        "cool job",
	StartedAt:   TestRunStartTime,
	CompletedAt: TestRunStartTime.Add(time.Minute*4 + time.Second*34),
	URL:         "https://github.com/jobs/10",
	RunID:       3,
	Steps: []Step{
		{
			Name:       "fob the barz",
			Status:     Completed,
			Conclusion: Success,
			Number:     1,
		},
		{
			Name:       "barz the fob",
			Status:     Completed,
			Conclusion: Success,
			Number:     2,
		},
	},
}

// Note that this run *has* steps, but in the ZIP archive the step logs are not
// included.
var SuccessfulJobWithoutStepLogs Job = Job{
	ID:          11,
	Status:      Completed,
	Conclusion:  Success,
	Name:        "cool job with no step logs",
	StartedAt:   TestRunStartTime,
	CompletedAt: TestRunStartTime.Add(time.Minute*4 + time.Second*34),
	URL:         "https://github.com/jobs/11",
	RunID:       3,
	Steps: []Step{
		{
			Name:       "fob the barz",
			Status:     Completed,
			Conclusion: Success,
			Number:     1,
		},
		{
			Name:       "barz the fob",
			Status:     Completed,
			Conclusion: Success,
			Number:     2,
		},
	},
}

// Note that this run *has* steps, but in the ZIP archive the step logs are not
// included.
var LegacySuccessfulJobWithoutStepLogs Job = Job{
	ID:          12,
	Status:      Completed,
	Conclusion:  Success,
	Name:        "legacy cool job with no step logs",
	StartedAt:   TestRunStartTime,
	CompletedAt: TestRunStartTime.Add(time.Minute*4 + time.Second*34),
	URL:         "https://github.com/jobs/12",
	RunID:       3,
	Steps: []Step{
		{
			Name:       "fob the barz",
			Status:     Completed,
			Conclusion: Success,
			Number:     1,
		},
		{
			Name:       "barz the fob",
			Status:     Completed,
			Conclusion: Success,
			Number:     2,
		},
	},
}

var FailedJob Job = Job{
	ID:          20,
	Status:      Completed,
	Conclusion:  Failure,
	Name:        "sad job",
	StartedAt:   TestRunStartTime,
	CompletedAt: TestRunStartTime.Add(time.Minute*4 + time.Second*34),
	URL:         "https://github.com/jobs/20",
	RunID:       1234,
	Steps: []Step{
		{
			Name:       "barf the quux",
			Status:     Completed,
			Conclusion: Success,
			Number:     1,
		},
		{
			Name:       "quux the barf",
			Status:     Completed,
			Conclusion: Failure,
			Number:     2,
		},
	},
}

// Note that this run *has* steps, but in the ZIP archive the step logs are not
// included.
var FailedJobWithoutStepLogs Job = Job{
	ID:          21,
	Status:      Completed,
	Conclusion:  Failure,
	Name:        "sad job with no step logs",
	StartedAt:   TestRunStartTime,
	CompletedAt: TestRunStartTime.Add(time.Minute*4 + time.Second*34),
	URL:         "https://github.com/jobs/21",
	RunID:       1234,
	Steps: []Step{
		{
			Name:       "barf the quux",
			Status:     Completed,
			Conclusion: Success,
			Number:     1,
		},
		{
			Name:       "quux the barf",
			Status:     Completed,
			Conclusion: Failure,
			Number:     2,
		},
	},
}

// Note that this run *has* steps, but in the ZIP archive the step logs are not
// included.
var LegacyFailedJobWithoutStepLogs Job = Job{
	ID:          22,
	Status:      Completed,
	Conclusion:  Failure,
	Name:        "legacy sad job with no step logs",
	StartedAt:   TestRunStartTime,
	CompletedAt: TestRunStartTime.Add(time.Minute*4 + time.Second*34),
	URL:         "https://github.com/jobs/22",
	RunID:       1234,
	Steps: []Step{
		{
			Name:       "barf the quux",
			Status:     Completed,
			Conclusion: Success,
			Number:     1,
		},
		{
			Name:       "quux the barf",
			Status:     Completed,
			Conclusion: Failure,
			Number:     2,
		},
	},
}

var SuccessfulJobAnnotations []Annotation = []Annotation{
	{
		JobName:   "cool job",
		Message:   "the job is happy",
		Path:      "blaze.py",
		Level:     "notice",
		StartLine: 420,
	},
}

var FailedJobAnnotations []Annotation = []Annotation{
	{
		JobName:   "sad job",
		Message:   "the job is sad",
		Path:      "blaze.py",
		Level:     "failure",
		StartLine: 420,
	},
}

var TestWorkflow workflowShared.Workflow = workflowShared.Workflow{
	Name: "CI",
	ID:   123,
}

type TestExporter struct {
	fields       []string
	writeHandler func(io *iostreams.IOStreams, data interface{}) error
}

func MakeTestExporter(fields []string, wh func(io *iostreams.IOStreams, data interface{}) error) *TestExporter {
	return &TestExporter{fields: fields, writeHandler: wh}
}

func (t *TestExporter) Fields() []string {
	return t.fields
}

func (t *TestExporter) Write(io *iostreams.IOStreams, data interface{}) error {
	return t.writeHandler(io, data)
}
