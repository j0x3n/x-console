// Package scheduler runs background jobs: fixed intervals and cron schedules.
// Cron specs use 5 fields and are evaluated in the user's time zone.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Job is one run of a scheduled task. Returning an error only logs it.
type Job func(ctx context.Context) error

// EntryID identifies a cron entry so it can be removed.
type EntryID = cron.EntryID

// Scheduler wraps robfig/cron with context handling and logging.
type Scheduler struct {
	c      *cron.Cron
	loc    *time.Location
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New builds a scheduler in loc.
func New(loc *time.Location) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{c: cron.New(cron.WithLocation(loc)), loc: loc, ctx: ctx, cancel: cancel}
}

// Location is the user's time zone.
func (s *Scheduler) Location() *time.Location { return s.loc }

// Cron adds a job with a 5-field cron spec, for example "0 8 * * *".
func (s *Scheduler) Cron(name, spec string, job Job) (EntryID, error) {
	return s.c.AddFunc(spec, s.wrap(name, job))
}

// Every adds a job that runs every d, starting after d.
func (s *Scheduler) Every(name string, d time.Duration, job Job) EntryID {
	return s.c.Schedule(cron.Every(d), cron.FuncJob(s.wrap(name, job)))
}

// Remove deletes a job.
func (s *Scheduler) Remove(id EntryID) { s.c.Remove(id) }

// Next returns when a cron spec fires next after t. Use it to validate user input.
func (s *Scheduler) Next(spec string, t time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(t.In(s.loc)), nil
}

// Start begins running jobs.
func (s *Scheduler) Start() { s.c.Start() }

// Stop stops scheduling and waits for running jobs.
func (s *Scheduler) Stop() {
	s.cancel()
	<-s.c.Stop().Done()
	s.wg.Wait()
}

func (s *Scheduler) wrap(name string, job Job) func() {
	return func() {
		s.wg.Add(1)
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("scheduled job panicked", "job", name, "panic", r)
			}
		}()
		if err := job(s.ctx); err != nil {
			slog.Error("scheduled job failed", "job", name, "err", err)
		}
	}
}
