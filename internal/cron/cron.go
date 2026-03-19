// Package cron provides scheduled task management for Cairn.
// Cron expressions use standard 5-field format (minute hour dom month dow).
package cron

import (
	"fmt"
	"time"

	cronlib "github.com/robfig/cron/v3"
)

// parser handles standard 5-field cron expressions.
var parser = cronlib.NewParser(
	cronlib.Minute | cronlib.Hour | cronlib.Dom | cronlib.Month | cronlib.Dow,
)

// Validate checks whether expr is a valid 5-field cron expression.
func Validate(expr string) error {
	_, err := parser.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return nil
}

// NextRun computes the next scheduled time after the given time.
func NextRun(expr string, after time.Time) (time.Time, error) {
	sched, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return sched.Next(after), nil
}

// IsDue returns true if the cron expression has a scheduled time between
// (after, now]. Used to check if a job should fire at the current tick.
func IsDue(expr string, after time.Time, now time.Time) (bool, error) {
	sched, err := parser.Parse(expr)
	if err != nil {
		return false, err
	}
	next := sched.Next(after)
	return !next.After(now), nil
}
