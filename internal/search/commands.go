package search

import (
	"context"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

type IndexParams struct {
	FolderID string `json:"folder_id,omitempty" desc:"Library folder to index; all folders when omitted"`
	Wait     bool   `json:"wait,omitempty" desc:"Run to completion before returning instead of indexing in the background"`
}

type IndexResult struct {
	Jobs []Progress `json:"jobs" desc:"One run per folder; offline folders are skipped"`
}

func (s *service) indexCmd(ctx context.Context, env *command.Env, p IndexParams) (IndexResult, error) {
	all, err := s.folders(ctx, env)
	if err != nil {
		return IndexResult{}, err
	}
	folders, err := pick(all, ids(p.FolderID)...)
	if err != nil {
		return IndexResult{}, err
	}
	res := IndexResult{Jobs: []Progress{}}
	for _, f := range folders {
		res.Jobs = append(res.Jobs, s.start(ctx, env, f, p.Wait))
	}
	return res, nil
}

func ids(id string) []string {
	if id == "" {
		return nil
	}
	return []string{id}
}

type StatusParams struct {
	FolderID string `json:"folder_id,omitempty" desc:"Only this library folder"`
}

type FolderStatus struct {
	FolderID  string    `json:"folder_id"`
	Label     string    `json:"label"`
	Online    bool      `json:"online"`
	Files     int       `json:"files" desc:"Book files in the folder"`
	Indexed   int       `json:"indexed" desc:"Files whose current version is in the index"`
	Failed    int       `json:"failed" desc:"Indexed files whose text could not be read"`
	Passages  int       `json:"passages" desc:"Searchable passages in the index"`
	Running   bool      `json:"running"`
	State     string    `json:"state" enum:"none,running,complete,cancelled,interrupted,failed" desc:"none: never indexed; interrupted: the app stopped mid-run"`
	LastError string    `json:"last_error,omitempty"`
	Job       *Progress `json:"job,omitempty" desc:"The latest run in this session"`
}

func (s *service) statusCmd(ctx context.Context, env *command.Env, p StatusParams) ([]FolderStatus, error) {
	all, err := s.folders(ctx, env)
	if err != nil {
		return nil, err
	}
	folders, err := pick(all, ids(p.FolderID)...)
	if err != nil {
		return nil, err
	}
	out := []FolderStatus{}
	for _, f := range folders {
		st := FolderStatus{FolderID: f.ID, Label: f.Label, Online: f.Online, State: "none"}
		files, err := libFiles(ctx, env.Store, f.ID)
		if err != nil {
			return nil, err
		}
		st.Files = len(files)
		if err := s.fillStatus(ctx, &st, files); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *service) fillStatus(ctx context.Context, st *FolderStatus, files []libFile) error {
	s.mu.Lock()
	j := s.jobs[st.FolderID]
	s.mu.Unlock()
	if j != nil {
		p := j.progress()
		st.Job, st.Running = &p, p.Running
	}
	ix, err := s.index(st.FolderID, false)
	if err != nil || ix == nil {
		return err
	}
	known, err := ix.indexed(ctx)
	if err != nil {
		return err
	}
	for _, f := range files {
		if current(known, f) {
			st.Indexed++
			if known[f.id].err != "" {
				st.Failed++
			}
		}
	}
	if err := ix.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM passages`).Scan(&st.Passages); err != nil {
		return err
	}
	st.State, st.LastError = ix.getState(ctx, "status"), ix.getState(ctx, "last_error")
	switch {
	case st.Running:
		st.State = "running"
	case st.State == "running":
		st.State = "interrupted"
	case st.State == "":
		st.State = "none"
	}
	return nil
}

type CancelParams struct {
	FolderID string `json:"folder_id,omitempty" desc:"Library folder to stop indexing; all when omitted"`
}

func (s *service) cancelCmd(ctx context.Context, env *command.Env, p CancelParams) (IndexResult, error) {
	s.mu.Lock()
	var jobs []*job
	for id, j := range s.jobs {
		if p.FolderID == "" || id == p.FolderID {
			jobs = append(jobs, j)
		}
	}
	s.mu.Unlock()
	res := IndexResult{Jobs: []Progress{}}
	for _, j := range jobs {
		j.cancel()
		<-j.done
		res.Jobs = append(res.Jobs, j.progress())
	}
	return res, nil
}

type RebuildParams struct {
	FolderID string `json:"folder_id" required:"true" desc:"Library folder whose index to rebuild"`
	Wait     bool   `json:"wait,omitempty" desc:"Run to completion before returning"`
}

func (s *service) rebuildCmd(ctx context.Context, env *command.Env, p RebuildParams) (Progress, error) {
	all, err := s.folders(ctx, env)
	if err != nil {
		return Progress{}, err
	}
	folders, err := pick(all, p.FolderID)
	if err != nil {
		return Progress{}, err
	}
	if !folders[0].Online {
		return Progress{}, command.Errorf("unavailable", "library folder %q is offline; its index is kept until it is back", folders[0].Label)
	}
	if _, err := s.cancelCmd(ctx, env, CancelParams{FolderID: p.FolderID}); err != nil {
		return Progress{}, err
	}
	if err := s.drop(p.FolderID); err != nil {
		return Progress{}, err
	}
	return s.start(ctx, env, folders[0], p.Wait), nil
}
