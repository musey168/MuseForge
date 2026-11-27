package repo

import (
	"fmt"
	"sort"
	"time"

	"agentflow/internal/domain"
)

// RepositoriesRepo owns the git repositories a project is connected to. A
// repository is a provider-agnostic remote: GitHub, GitLab or a plain HTTP(S)
// host. The personal build keeps them — a requirement links to one — but
// nothing dispatches work through them.
type RepositoriesRepo struct {
	*deps
	projects *ProjectsRepo
}

// Repositories lists every repository, grouped by project and named.
func (r *RepositoriesRepo) Repositories() []domain.Repository {
	out := payloadList[domain.Repository](r.DB, "repositories", "")
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectID == out[j].ProjectID {
			return out[i].Name < out[j].Name
		}
		return out[i].ProjectID < out[j].ProjectID
	})
	return out
}

// Repository reads one repository.
func (r *RepositoriesRepo) Repository(id string) (domain.Repository, bool) {
	return payloadOne[domain.Repository](r.DB, "repositories", id)
}

// PutRepository stores a repository. It must belong to an existing project.
func (r *RepositoriesRepo) PutRepository(v domain.Repository) error {
	if _, ok := r.projects.Project(v.ProjectID); !ok {
		return fmt.Errorf("project not found")
	}
	now := time.Now().UTC()
	if old, ok := r.Repository(v.ID); ok {
		v.CreatedAt = old.CreatedAt
	} else {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	return upsert(r.DB, "repositories", map[string]any{
		"id": v.ID, "project_id": v.ProjectID, "name": v.Name, "payload": mustJSON(v),
		"created_at": timePtr(v.CreatedAt), "updated_at": timePtr(v.UpdatedAt),
	})
}

// DeleteRepository removes a repository.
func (r *RepositoriesRepo) DeleteRepository(id string) error {
	return r.DB.Table("repositories").Where("id = ?", id).Delete(nil).Error
}
