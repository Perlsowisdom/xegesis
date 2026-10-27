package search

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Progress is one indexing run's state.
type Progress struct {
	JobID     string `json:"job_id"`
	FolderID  string `json:"folder_id"`
	Running   bool   `json:"running"`
	Files     int    `json:"files" desc:"Book files in the folder"`
	ToIndex   int    `json:"to_index" desc:"New or changed files this run must read"`
	Indexed   int    `json:"indexed" desc:"Files read and indexed so far in this run"`
	Failed    int    `json:"failed" desc:"Files whose text could not be read; they are retried when they change"`
	Unchanged int    `json:"unchanged" desc:"Files already indexed in their current version"`
	Removed   int    `json:"removed" desc:"Files dropped from the index because they left the library"`
	State     string `json:"state" enum:"running,complete,cancelled,failed,skipped"`
	Error     string `json:"error,omitempty"`
	StartedAt int64  `json:"started_at,omitempty"`
	EndedAt   int64  `json:"ended_at,omitempty"`
}

type job struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	p      Progress
}

func (j *job) progress() Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.p
}

func (j *job) update(fn func(*Progress)) {
	j.mu.Lock()
	fn(&j.p)
	j.mu.Unlock()
}

// start begins indexing a folder, or returns the run already under way. With wait it returns
// when the run ends, and cancelling ctx stops it.
func (s *service) start(ctx context.Context, env *command.Env, f library.Folder, wait bool) Progress {
	if !f.Online {
		return Progress{FolderID: f.ID, State: "skipped", Error: fmt.Sprintf("folder %q is offline", f.Label)}
	}
	s.mu.Lock()
	j := s.jobs[f.ID]
	mine := j == nil || !j.progress().Running
	if mine {
		base := context.Background()
		if wait {
			base = ctx
		}
		jctx, cancel := context.WithCancel(base)
		j = &job{cancel: cancel, done: make(chan struct{}),
			p: Progress{JobID: store.NewID(), FolderID: f.ID, Running: true, State: "running", StartedAt: now()}}
		s.jobs[f.ID] = j
		go func() {
			defer close(j.done)
			defer cancel()
			s.run(jctx, env.Store, j, f)
		}()
	}
	s.mu.Unlock()
	if wait {
		select {
		case <-j.done:
		case <-ctx.Done():
			if mine {
				<-j.done // the run was started with ctx, so it is stopping
			}
		}
	}
	return j.progress()
}

func now() int64 { return time.Now().UnixMilli() }

// run indexes the folder's new and changed files and drops those that left the library.
func (s *service) run(ctx context.Context, st *store.Store, j *job, f library.Folder) {
	err := s.indexFolder(ctx, st, j, f)
	state, msg, ended := "complete", "", now()
	switch {
	case errors.Is(err, context.Canceled) || ctx.Err() != nil:
		state = "cancelled"
	case err != nil:
		state, msg = "failed", err.Error()
	}
	if ix, _ := s.index(f.ID, false); ix != nil {
		ix.setState(context.Background(), map[string]string{"status": state, "last_error": msg,
			"ended_at": strconv.FormatInt(ended, 10)})
	}
	j.update(func(p *Progress) { p.Running, p.State, p.Error, p.EndedAt = false, state, msg, ended })
}

func (s *service) indexFolder(ctx context.Context, st *store.Store, j *job, f library.Folder) error {
	ix, err := s.index(f.ID, true)
	if err != nil {
		return err
	}
	files, err := libFiles(ctx, st, f.ID)
	if err != nil {
		return err
	}
	known, err := ix.indexed(ctx)
	if err != nil {
		return err
	}
	if err := ix.setState(ctx, map[string]string{"status": "running", "job_id": j.p.JobID, "last_error": ""}); err != nil {
		return err
	}
	live := map[string]bool{}
	var todo []libFile
	for _, lf := range files {
		live[lf.id] = true
		if !current(known, lf) {
			todo = append(todo, lf)
		}
	}
	removed := 0
	for id := range known {
		if !live[id] {
			if err := ix.remove(ctx, id); err != nil {
				return err
			}
			removed++
		}
	}
	j.update(func(p *Progress) {
		p.Files, p.ToIndex, p.Unchanged, p.Removed = len(files), len(todo), len(files)-len(todo), removed
	})
	return s.indexFiles(ctx, ix, j, f, todo)
}

type extracted struct {
	f   libFile
	ps  []passage
	err error
}

// indexFiles reads files on a few workers and writes each to the index as soon as it is read.
func (s *service) indexFiles(ctx context.Context, ix *index, j *job, f library.Folder, todo []libFile) error {
	in := make(chan libFile)
	out := make(chan extracted)
	var wg sync.WaitGroup
	for range s.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for lf := range in {
				ps, err := readBook(filepath.Join(f.Path, filepath.FromSlash(lf.rel)), lf.format)
				select {
				case out <- extracted{lf, ps, err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(in)
		for _, lf := range todo {
			select {
			case in <- lf:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(out) }()
	var failed error
	for e := range out {
		if failed != nil || ctx.Err() != nil {
			continue
		}
		if err := ix.put(ctx, e.f, e.ps, e.err); err != nil {
			failed = err
			continue
		}
		j.update(func(p *Progress) {
			p.Indexed++
			if e.err != nil {
				p.Failed++
			}
		})
		if s.onFile != nil {
			s.onFile(f.ID)
		}
	}
	if failed != nil {
		return failed
	}
	return ctx.Err()
}

// readBook extracts a file's passages. A file the extractors cannot cope with is an error, not a
// crash.
func readBook(path, format string) (ps []passage, err error) {
	defer func() {
		if x := recover(); x != nil {
			ps, err = nil, fmt.Errorf("cannot read %s: %v", filepath.Base(path), x)
		}
	}()
	sections, err := extractFile(path, format)
	if err != nil {
		return nil, err
	}
	return chunk(sections), nil
}
