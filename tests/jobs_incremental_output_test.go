//

package tests

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/jobs"
)

func TestJobsIncrementalOutput(t *testing.T) {
	var argv []string
	if runtime.GOOS == "windows" {
		argv = []string{"powershell.exe", "-NoProfile", "-Command", "1..10 | ForEach-Object { Write-Output $_ ; Start-Sleep -Milliseconds 100 }"}
	} else {
		argv = []string{"sh", "-c", "for i in 1 2 3 4 5 6 7 8 9 10; do echo $i; sleep 0.1; done"}
	}
	rt := jobs.NewRuntime()
	job, err := rt.StartSubprocess(jobs.SubprocessSpec{
		Kind:           jobs.KindBash,
		Label:          "seq 1 10",
		OwnerSession:   brand.NewSessionID("owner1"),
		Argv:           argv,
		Cwd:            t.TempDir(),
		StdoutMaxBytes: 65536,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for i := 0; i < 50; i++ {
		if t.Failed() {
			break
		}
		got.WriteString(job.ReadOutput())
		select {
		case out := <-job.Done():
			got.WriteString(job.ReadOutput())
			if out.Status != jobs.StatusCompleted {
				t.Fatalf("should be completed, got %+v", out)
			}
		default:
		}
		if strings.Contains(got.String(), "10") {
			break
		}
		<-time.After(40 * time.Millisecond)
	}
	if !strings.Contains(got.String(), "10") {
		t.Fatalf("should read final output 10, got %q", got.String())
	}
	if job.ID == (brand.JobID{}) {
		t.Fatal("should have job id")
	}
}

func TestJobsListByOwner(t *testing.T) {
	rt := jobs.NewRuntime()
	for i := 0; i < 3; i++ {
		_, _ = rt.Start(manualStart(jobs.KindBash, "ownerX", func() {}))
	}
	_, _ = rt.Start(manualStart(jobs.KindBash, "ownerY", func() {}))
	if got := len(rt.ListByOwner(brand.NewSessionID("ownerX"))); got != 3 {
		t.Fatalf("ownerX should have 3 jobs, got %d", got)
	}
}

func TestJobsOwnerDisposeCancels(t *testing.T) {
	owner := brand.NewSessionID("owner-z")
	rt := jobs.NewRuntime()
	cancelled := make(chan struct{}, 2)

	_, _ = rt.Start(manualStart(jobs.KindBash, owner.Raw(), func() { cancelled <- struct{}{} }))
	_, _ = rt.Start(manualStart(jobs.KindBash, owner.Raw(), func() { cancelled <- struct{}{} }))

	done := make(chan struct{})
	go func() {
		rt.DisposeOwner(owner, "owner disposed")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("DisposeOwner did not complete within timeout")
	}

	count := 0
	for i := 0; i < 2; i++ {
		select {
		case <-cancelled:
			count++
		case <-time.After(time.Second):
		}
	}
	if count != 2 {
		t.Fatalf("dispose should cancel all 2 jobs, got %d", count)
	}

	if jobs := rt.ListByOwner(owner); len(jobs) != 0 {
		t.Fatalf("owner list should be empty after dispose, got %d", len(jobs))
	}
}

func TestJobsOwnerDisposeSubprocessCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("depends on sh")
	}
	owner := brand.NewSessionID("owner-sp")
	rt := jobs.NewRuntime()
	_, err := rt.StartSubprocess(jobs.SubprocessSpec{
		Kind:         jobs.KindBash,
		Label:        "sleep long",
		OwnerSession: owner,
		Argv:         []string{"sh", "-c", "sleep 1000"},
		Cwd:          t.TempDir(),
		GraceMs:      100,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { rt.DisposeOwner(owner, "dispose"); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("dispose subprocess job timed out")
	}
	if len(rt.ListByOwner(owner)) != 0 {
		t.Fatal("subprocess job should be removed from registry after dispose")
	}
}

func manualStart(kind jobs.JobKind, ownerID string, onCancel func()) jobs.JobStart {
	owner := brand.NewSessionID(ownerID)
	done := make(chan jobs.JobOutcome, 1)
	return jobs.JobStart{
		Kind:         kind,
		Label:        "manual",
		OwnerSession: owner,
		Run: func() *jobs.JobHooks {
			return &jobs.JobHooks{

				Cancel: func(reason string) {
					onCancel()
					select {
					case done <- jobs.JobOutcome{Status: jobs.StatusKilled, Detail: "cancelled"}:
					default:
					}
				},
				Done:       done,
				ReadOutput: func() string { return "" },
			}
		},
	}
}
