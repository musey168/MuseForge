package repo

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"agentflow/internal/domain"

	"gorm.io/gorm"
)

var (
	ErrGitHTTPCredentialExists = errors.New("该主机已有访问令牌，请编辑现有凭据")
	ErrGitHTTPTokenRequired    = errors.New("添加凭据时请填写访问令牌")
)

func gitHTTPCredentialID(host string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(host)))
}

func (row gitHTTPCredentialRow) secret() domain.GitHTTPCredentialSecret {
	host := row.Host
	if row.HTTPHost != "" {
		host = row.HTTPHost
	}
	return domain.GitHTTPCredentialSecret{
		GitHTTPCredential: domain.GitHTTPCredential{
			ID: row.ID, Name: row.Name, Platform: row.Platform, Host: host,
			Username: row.Username},
		Token: row.Token,
	}
}

// GitHTTPCredentials describes the user's HTTP(S) Git credentials.
// Tokens are never read into the description.
func (r *UsersRepo) GitHTTPCredentials(userID int64) ([]domain.GitHTTPCredential, error) {
	var rows []gitHTTPCredentialRow
	err := r.DB.Select("id", "name", "platform", "host", "http_host", "username").
		Where("user_id = ?", userID).Order("name, id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.GitHTTPCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.secret().GitHTTPCredential)
	}
	return out, nil
}

// SaveGitHTTPCredential creates the credential for input.Host when existingID
// is empty, otherwise updates that credential; an empty token keeps the
// stored one. The host of an existing credential cannot change.
func (r *UsersRepo) SaveGitHTTPCredential(userID int64, existingID string, input domain.GitHTTPCredentialSecret) error {
	endpoint, err := domain.ParseGitHTTPEndpoint(input.Host)
	if err != nil {
		return err
	}
	host, httpHost := endpoint.Scope(), ""
	id := gitHTTPCredentialID(host)
	if endpoint.Scheme == "http" {
		httpHost, host = host, ""
		input.Platform = domain.GitPlatformOther
	}
	now := time.Now().UTC()
	if existingID == "" {
		if input.Token == "" {
			return ErrGitHTTPTokenRequired
		}
		row := gitHTTPCredentialRow{
			UserID: userID, ID: id, Name: input.Name, Platform: input.Platform, Host: host, HTTPHost: httpHost,
			Username: input.Username, Token: input.Token, Created: &now, Updated: &now,
		}
		err := r.DB.Create(&row).Error
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrGitHTTPCredentialExists
		}
		return err
	}
	if existingID != id {
		return gorm.ErrRecordNotFound
	}
	row := r.DB.Model(&gitHTTPCredentialRow{}).Where("user_id = ? AND id = ?", userID, id)
	updates := map[string]any{
		"name": input.Name, "platform": input.Platform, "username": input.Username,
		"updated_at": now,
	}
	if input.Token != "" {
		updates["token"] = input.Token
	}
	result := row.Session(&gorm.Session{}).Updates(updates)
	if result.Error != nil || result.RowsAffected != 0 {
		return result.Error
	}
	// MySQL can report zero affected rows for an unchanged value.
	var count int64
	if err := row.Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *UsersRepo) DeleteGitHTTPCredential(userID int64, id string) error {
	return r.DB.Where("user_id = ? AND id = ?", userID, id).Delete(&gitHTTPCredentialRow{}).Error
}

// GitHTTPCredentialForHost returns the user's credential for one HTTP(S)
// endpoint, refreshing a managed OAuth token when applicable.
func (r *UsersRepo) GitHTTPCredentialForHost(ctx context.Context, userID int64, host string) (domain.GitHTTPCredentialSecret, bool) {
	endpoint, err := domain.ParseGitHTTPEndpoint(host)
	if err != nil || userID <= 0 {
		return domain.GitHTTPCredentialSecret{}, false
	}
	host = endpoint.Scope()
	var row gitHTTPCredentialRow
	if err := r.DB.WithContext(ctx).Where("user_id = ? AND id = ?", userID, gitHTTPCredentialID(host)).Limit(1).Find(&row).Error; err != nil {
		return domain.GitHTTPCredentialSecret{}, false
	}
	secret := row.secret()
	if row.ID != "" && secret.Host == host && secret.Token != "" {
		return secret, true
	}
	return domain.GitHTTPCredentialSecret{}, false
}

// GitHTTPCredentialSecrets is used only by authenticated execution dispatch.
// Public settings and run snapshots continue to use credential descriptions.
func (r *UsersRepo) GitHTTPCredentialSecrets(ctx context.Context, userID int64) map[string]domain.GitHTTPExecutionCredential {
	out := map[string]domain.GitHTTPExecutionCredential{}
	credentials, err := r.GitHTTPCredentials(userID)
	if err != nil {
		return out
	}
	for _, credential := range credentials {
		if secret, ok := r.GitHTTPCredentialForHost(ctx, userID, credential.Host); ok {
			out[credential.Host] = domain.GitHTTPExecutionCredential{
				GitHTTPCredential: secret.GitHTTPCredential, Token: secret.Token, ExpiresAt: secret.ExpiresAt,
			}
		}
	}
	return out
}

// GitHTTPCredentialByID resolves one listed credential for verification.
func (r *UsersRepo) GitHTTPCredentialByID(ctx context.Context, userID int64, id string) (domain.GitHTTPCredentialSecret, bool) {
	var row gitHTTPCredentialRow
	if err := r.DB.Where("user_id = ? AND id = ?", userID, id).Limit(1).Find(&row).Error; err != nil || row.ID == "" {
		return domain.GitHTTPCredentialSecret{}, false
	}
	return row.secret(), true
}

// GitHTTPWorkspaceHosts maps each host whose credential the user hands to
// workspace turns onto its platform. It carries no tokens: a turn requests
// the token for one host when it needs it.
func (r *UsersRepo) GitHTTPWorkspaceHosts(userID int64) map[string]string {
	if userID <= 0 {
		return nil
	}
	creds, err := r.GitHTTPCredentials(userID)
	if err != nil {
		return nil
	}
	hosts := make(map[string]string, len(creds))
	for _, cred := range creds {
		hosts[cred.Host] = cred.Platform
	}
	return hosts
}
