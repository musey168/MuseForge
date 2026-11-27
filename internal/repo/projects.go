package repo

import (
	"sort"
	"time"

	"agentflow/internal/domain"
	"gorm.io/gorm"
)

// ---------- Projects ----------

func (r *ProjectsRepo) lockProjectInTx(tx *gorm.DB, id string) error {
	var row projectRow
	return r.locking(tx).Select("id").Where("id = ?", id).First(&row).Error
}

// Projects lists live projects; deleted (archived) ones are left out. Project
// still returns an archived project so its history and restore keep working.
func (r *ProjectsRepo) Projects() []domain.Project {
	var rows []projectRow
	if err := r.DB.Find(&rows).Error; err != nil {
		return []domain.Project{}
	}
	out := make([]domain.Project, 0, len(rows))
	for _, row := range rows {
		var project domain.Project
		if jsonUnmarshal(row.Payload, &project) != nil || project.ArchivedAt != nil {
			continue
		}
		if row.OwnerUserID != nil {
			project.OwnerID = *row.OwnerUserID
		} else {
			project.OwnerID = 0
		}
		if row.Updated != nil {
			project.UpdatedAt = *row.Updated
		}
		out = append(out, project)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *ProjectsRepo) Project(id string) (domain.Project, bool) {
	project, err := r.projectWithOwnerInTx(r.DB, id)
	return project, err == nil
}

func (r *ProjectsRepo) projectWithOwnerInTx(tx *gorm.DB, id string) (domain.Project, error) {
	var row projectRow
	if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
		return domain.Project{}, err
	}
	var project domain.Project
	if err := jsonUnmarshal(row.Payload, &project); err != nil {
		return domain.Project{}, err
	}
	if row.OwnerUserID != nil {
		project.OwnerID = *row.OwnerUserID
	} else {
		project.OwnerID = 0
	}
	if row.Updated != nil {
		project.UpdatedAt = *row.Updated
	}
	return project, nil
}

// ProjectOwners returns only project identity and ownership for batched
// authorization. Missing projects are absent from the map.
func (r *ProjectsRepo) ProjectOwners(ids []string) (map[string]domain.Project, error) {
	out := make(map[string]domain.Project, len(ids))
	for start := 0; start < len(ids); start += 500 {
		batch := ids[start:min(start+500, len(ids))]
		var rows []projectRow
		if err := r.DB.Select("id", "owner_user_id").Where("id IN ?", batch).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			project := domain.Project{ID: row.ID}
			if row.OwnerUserID != nil {
				project.OwnerID = *row.OwnerUserID
			}
			out[row.ID] = project
		}
	}
	return out, nil
}

// SetProjectArchived deletes (at != nil) or restores (at == nil) a project
// without touching its settings or requirements.
func (r *ProjectsRepo) SetProjectArchived(id string, at *time.Time, by int64) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		var row projectRow
		if err := r.locking(tx).Where("id = ?", id).Take(&row).Error; err != nil {
			return err
		}
		payload := map[string]any{}
		if err := jsonUnmarshal(row.Payload, &payload); err != nil {
			return err
		}
		if at != nil {
			payload["archivedAt"] = at.UTC()
			payload["archivedBy"] = by
		} else {
			delete(payload, "archivedAt")
			delete(payload, "archivedBy")
		}
		return tx.Model(&projectRow{}).Where("id = ?", id).Updates(map[string]any{
			"payload": mustJSON(payload), "updated_at": timePtr(time.Now().UTC()),
		}).Error
	})
}

// PutProject saves the project.
func (r *ProjectsRepo) PutProject(v domain.Project) error {
	return r.writeProject(v)
}

func (r *ProjectsRepo) writeProject(v domain.Project) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		var current projectRow
		err := r.locking(tx).Where("id = ?", v.ID).Take(&current).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		now := time.Now().UTC()
		v.CreatedAt, v.UpdatedAt = now, now
		if err == nil {
			var ownerID int64
			if current.OwnerUserID != nil {
				ownerID = *current.OwnerUserID
			}
			if v.OwnerID == 0 {
				v.OwnerID = ownerID
			}
			if current.Created != nil {
				v.CreatedAt = *current.Created
			}
		}
		// The RSI settings endpoint owns this separate project setting.
		v.SOPDetection = nil
		if current.ID != "" {
			var stored domain.Project
			if err := jsonUnmarshal(current.Payload, &stored); err != nil {
				return err
			}
			v.SOPDetection = stored.SOPDetection
		}
		payload := v
		payload.GitToken, payload.CodeBuddyAPIKey = "", ""
		var owner any
		if v.OwnerID > 0 {
			owner = v.OwnerID
		}
		if err := upsert(tx, "projects", map[string]any{
			"id": v.ID, "project_key": v.Key, "name": v.Name, "owner_user_id": owner,
			"payload": mustJSON(payload), "created_at": timePtr(v.CreatedAt), "updated_at": timePtr(v.UpdatedAt),
		}); err != nil {
			return err
		}
		if v.OwnerID > 0 {
			// Backfill issue responsibility for issues that have no assignee.
			var issueRows []issueRow
			if err := tx.Where("project_id = ? AND assignee_user_id IS NULL", v.ID).Find(&issueRows).Error; err != nil {
				return err
			}
			for _, row := range issueRows {
				var issue domain.Issue
				if jsonUnmarshal(row.Payload, &issue) != nil {
					continue
				}
				issue.AssigneeUserID = v.OwnerID
				if err := tx.Model(&issueRow{}).Where("id = ?", row.ID).Updates(map[string]any{
					"assignee_user_id": v.OwnerID, "payload": mustJSON(issue),
				}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
