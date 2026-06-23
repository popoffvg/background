package store

import (
	"os"
	"sort"
	"time"
)

// Default GC thresholds. Used by the launch sweep and by `bg gc` when a flag is
// omitted. Overridable via env vars (see GCDefaults).
const (
	DefaultGCMaxAge        = 14 * 24 * time.Hour // purge out.log older than this
	DefaultGCMaxTotalBytes = 500 * 1024 * 1024   // total out.log budget
	DefaultGCHardAge       = 30 * 24 * time.Hour // remove whole task dir
	DefaultGCMaxTasks      = 0                   // 0 = disabled
)

// GCOptions configures a garbage-collection pass. A zero value for a field
// disables that rule.
type GCOptions struct {
	MaxAge        time.Duration // age purge: delete out.log of finished tasks older than this
	MaxTotalBytes int64         // size budget over all remaining out.logs
	HardAge       time.Duration // hard delete: remove whole dir of finished tasks older than this
	MaxTasks      int           // hard delete oldest finished tasks beyond this count
	DryRun        bool          // report only; make no changes
}

// GCResult reports what a GC pass did (or would do, when DryRun).
type GCResult struct {
	LogsPurged     int   // out.log files deleted (task dir kept)
	DirsDeleted    int   // whole task dirs removed
	BytesReclaimed int64 // total bytes freed
}

// GCDefaults returns the default options, applying env overrides:
// BG_GC_MAX_AGE, BG_GC_MAX_SIZE, BG_GC_HARD_AGE, BG_GC_MAX_TASKS.
func GCDefaults() GCOptions {
	o := GCOptions{
		MaxAge:        DefaultGCMaxAge,
		MaxTotalBytes: DefaultGCMaxTotalBytes,
		HardAge:       DefaultGCHardAge,
		MaxTasks:      DefaultGCMaxTasks,
	}
	if v := os.Getenv("BG_GC_MAX_AGE"); v != "" {
		if d, err := ParseDuration(v); err == nil {
			o.MaxAge = d
		}
	}
	if v := os.Getenv("BG_GC_MAX_SIZE"); v != "" {
		if b, err := ParseBytes(v); err == nil {
			o.MaxTotalBytes = b
		}
	}
	if v := os.Getenv("BG_GC_HARD_AGE"); v != "" {
		if d, err := ParseDuration(v); err == nil {
			o.HardAge = d
		}
	}
	if v := os.Getenv("BG_GC_MAX_TASKS"); v != "" {
		if n, err := ParseInt(v); err == nil {
			o.MaxTasks = n
		}
	}
	return o
}

// taskAge returns the reference time for retention: FinishedAt if present, else
// StartedAt. The bool reports whether a usable time was found.
func taskAge(t Task) (time.Time, bool) {
	if st, exists, err := ReadStatus(t.Dir); err == nil && exists && st.FinishedAt != "" {
		if ts, err := time.Parse(time.RFC3339, st.FinishedAt); err == nil {
			return ts, true
		}
	}
	if t.Meta.StartedAt != "" {
		if ts, err := time.Parse(time.RFC3339, t.Meta.StartedAt); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

// logSize returns the byte size of a task's out.log (0 if absent).
func logSize(t Task) int64 {
	fi, err := os.Stat(t.LogPath())
	if err != nil {
		return 0
	}
	return fi.Size()
}

// GC runs a garbage-collection pass over FINISHED tasks only. Running tasks are
// never touched. Rules apply in order: age purge → size budget → hard delete →
// max-tasks. Returns counts of what was (or would be) reclaimed.
func GC(opts GCOptions) (GCResult, error) {
	var res GCResult
	all, err := ListTasks()
	if err != nil {
		return res, err
	}

	// Finished tasks only, sorted oldest-first by reference time.
	type aged struct {
		task Task
		ref  time.Time
	}
	var finished []aged
	for _, t := range all {
		if t.State() == "running" {
			continue
		}
		ref, ok := taskAge(t)
		if !ok {
			continue
		}
		finished = append(finished, aged{task: t, ref: ref})
	}
	sort.Slice(finished, func(i, j int) bool {
		return finished[i].ref.Before(finished[j].ref)
	})

	now := time.Now()
	// Track which dirs still exist after each phase.
	removedDir := make(map[string]bool)
	purgedLog := make(map[string]bool)

	purgeLog := func(t Task) {
		if purgedLog[t.Dir] || removedDir[t.Dir] || !t.LogExists() {
			return
		}
		sz := logSize(t)
		if !opts.DryRun {
			if err := os.Remove(t.LogPath()); err != nil {
				return
			}
		}
		purgedLog[t.Dir] = true
		res.LogsPurged++
		res.BytesReclaimed += sz
	}

	removeDir := func(t Task) {
		if removedDir[t.Dir] {
			return
		}
		sz := logSize(t)
		if !opts.DryRun {
			if err := os.RemoveAll(t.Dir); err != nil {
				return
			}
		}
		removedDir[t.Dir] = true
		res.DirsDeleted++
		// If the log wasn't already counted as purged, count its bytes now.
		if !purgedLog[t.Dir] {
			res.BytesReclaimed += sz
		}
	}

	// 1. Age purge: delete out.log of finished tasks older than MaxAge.
	if opts.MaxAge > 0 {
		for _, a := range finished {
			if now.Sub(a.ref) > opts.MaxAge {
				purgeLog(a.task)
			}
		}
	}

	// 2. Size budget: while total remaining out.log bytes exceed MaxTotalBytes,
	//    delete the oldest finished task's out.log first.
	if opts.MaxTotalBytes > 0 {
		total := int64(0)
		for _, a := range finished {
			if removedDir[a.task.Dir] || purgedLog[a.task.Dir] {
				continue
			}
			total += logSize(a.task)
		}
		for _, a := range finished { // oldest-first
			if total <= opts.MaxTotalBytes {
				break
			}
			if removedDir[a.task.Dir] || purgedLog[a.task.Dir] {
				continue
			}
			sz := logSize(a.task)
			purgeLog(a.task)
			if purgedLog[a.task.Dir] {
				total -= sz
			}
		}
	}

	// 3. Hard delete: remove whole dir of finished tasks older than HardAge.
	if opts.HardAge > 0 {
		for _, a := range finished {
			if now.Sub(a.ref) > opts.HardAge {
				removeDir(a.task)
			}
		}
	}

	// 4. MaxTasks: hard-delete oldest finished tasks beyond the limit.
	if opts.MaxTasks > 0 {
		// Count survivors (dirs not removed), oldest-first they are already.
		var survivors []aged
		for _, a := range finished {
			if !removedDir[a.task.Dir] {
				survivors = append(survivors, a)
			}
		}
		excess := len(survivors) - opts.MaxTasks
		for i := 0; i < excess && i < len(survivors); i++ {
			removeDir(survivors[i].task)
		}
	}

	return res, nil
}
