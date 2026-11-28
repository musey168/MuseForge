package tracker

import (
	"agentflow/internal/domain"
)

// projectAuthFixture is the in-memory store the tracker tests use: the
// projects, repositories, users and Git credentials the import reads.
type projectAuthFixture struct {
	credentials map[int64]map[string]domain.GitHTTPCredentialSecret
	projects    map[string]domain.Project
	users       map[int64]domain.User
	repos       []domain.Repository
}

func (f projectAuthFixture) Projects() []domain.Project {
	out := make([]domain.Project, 0, len(f.projects))
	for _, project := range f.projects {
		out = append(out, project)
	}
	return out
}

func (f projectAuthFixture) Project(id string) (domain.Project, bool) {
	project, ok := f.projects[id]
	return project, ok
}

func (f projectAuthFixture) Repositories() []domain.Repository { return f.repos }

func (f projectAuthFixture) User(id int64) (domain.User, bool) {
	user, ok := f.users[id]
	return user, ok
}
