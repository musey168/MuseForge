package httpapi

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentflow/internal/domain"
	"agentflow/internal/gitwork"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)



func TestPlatformSSHRevocationPreservesRunnerLocalPin(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("OpenSSH client is not installed")
	}
	remote, privateKey, _, localFile := localSSHGit(t, nil)
	currentPin, err := os.ReadFile(localFile)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := gitwork.ParseSSHRemote(remote)
	if err != nil {
		t.Fatal(err)
	}
	_, oldKey := sshTestIdentity(t)
	oldPin := knownhosts.Line([]string{endpoint.Endpoint()}, oldKey) + "\n"
	keys := map[string]string{remote: privateKey}
	for _, tc := range []struct {
		name        string
		revokedPin  string
		wantFailure string
	}{
		{name: "old key revoked", revokedPin: oldPin},
		{name: "current key revoked", revokedPin: string(currentPin), wantFailure: "REVOKED HOST KEY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			platformFile := filepath.Join(t.TempDir(), "platform_known_hosts")
			if err := os.WriteFile(platformFile, []byte("@revoked "+tc.revokedPin), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(gitwork.SSHKnownHostsEnv, platformFile)
			pins := gitwork.SSHKnownHostsForKeys(t.Context(), keys)
			t.Setenv(gitwork.SSHKnownHostsEnv, localFile)
			ctx := gitwork.WithSSHKnownHosts(t.Context(), pins)
			ctx = gitwork.WithSSHRemote(gitwork.WithSSHKeys(ctx, keys), remote)
			_, err := gitwork.Run(ctx, t.TempDir(), "ls-remote", "--", remote)
			if tc.wantFailure == "" {
				if err != nil {
					t.Fatalf("runner-local replacement key was rejected: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantFailure) {
				t.Fatalf("OpenSSH did not reject the revoked key: %v", err)
			}
		})
	}
}



func TestGitSSHRunnerClaimUsesTaskActorCredential(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	actor := domain.User{LoginName: "ssh-delivery-actor"}
	if err := f.s.PutUser(actor); err != nil {
		t.Fatal(err)
	}
	actor, _ = f.s.UserByLoginName(actor.LoginName)
	const key = "transient-other-host-identity"
	const otherHostRepo = "git@git.example.org:team/app.git"
	gitlabRepo, gitlabKey, bare, knownHosts := localSSHGit(t, nil)
	expectedPin, err := os.ReadFile(knownHosts)
	if err != nil {
		t.Fatal(err)
	}
	_, unrelatedKey := sshTestIdentity(t)
	unrelatedPin := knownhosts.Line([]string{"unrelated.example.org:22"}, unrelatedKey) + "\n"
	if err := os.WriteFile(knownHosts, append(expectedPin, unrelatedPin...), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(gitwork.SSHKnownHostsEnv, knownHosts)
	if err := f.s.SaveGitSSHKey(actor.ID, "", otherHostRepo, "Other Git host", key); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SaveGitSSHKey(actor.ID, "", gitlabRepo, "GitLab", gitlabKey); err != nil {
		t.Fatal(err)
	}
	runnerID, session := registerTestRunner(t, f.h)
	registered, _ := f.s.Runner(runnerID)
	now := time.Now().UTC()
	if err := f.s.RunnerHeartbeat(runnerID, registered.Runtimes, domain.RunnerVersionGitSSHKnownHosts, false, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	task := domain.RunnerTask{ID: "ssh-task", RunnerID: runnerID, Provider: "mock", Status: domain.RunnerTaskQueued, MaxAttempts: 1, Invocation: &domain.RunnerInvocation{
		GitSSHUserID: actor.ID,
		Repositories: []domain.RunnerRepository{{CloneURL: gitlabRepo, UseSSH: true}, {CloneURL: otherHostRepo, UseSSH: true}},
	}}
	if err := f.s.PutRunnerTask(task); err != nil {
		t.Fatal(err)
	}
	auth := map[string]string{"Authorization": "Bearer " + session}
	resp := requestWithHeaders(t, f.h, http.MethodPost, "/api/runners/tasks/claim", map[string]any{
		"availableRuntimes":        []string{"mock"},
		"noWait":                   true,
		"supportsGitSshKnownHosts": true,
	}, auth)
	var result struct{ Task domain.RunnerTask }
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil || resp.Code != http.StatusOK || result.Task.ID != task.ID || result.Task.Invocation == nil || result.Task.Invocation.GitSSHKeys[gitlabRepo] != gitlabKey || result.Task.Invocation.GitSSHKeys[otherHostRepo] != key || result.Task.Invocation.GitSSHUserID != 0 {
		t.Fatalf("authenticated claim did not hydrate the task actor's keys (HTTP %d)", resp.Code)
	}
	pins := result.Task.GitSSHKnownHosts
	if pins == nil || pins.Entries != string(expectedPin) || pins.ReadFailed || resp.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("claim did not return just the requested platform host pin with no-store")
	}
	stored, _ := f.s.RunnerTask(task.ID)
	if stored.Invocation == nil || len(stored.Invocation.GitSSHKeys) != 0 || stored.GitSSHKnownHosts != nil {
		t.Fatal("claimed Git credentials or host pins were persisted")
	}
	if resp := requestWithHeaders(t, f.h, http.MethodGet, "/api/runners/tasks/"+task.ID, nil, auth); resp.Code != http.StatusOK || strings.Contains(resp.Body.String(), "gitSsh") || strings.Contains(resp.Body.String(), key) {
		t.Fatalf("runner status response must omit credential fields (HTTP %d)", resp.Code)
	}

	// A claim from an older client carries no capability flag, even if the
	// persisted runner version was reported by a different process.
	task.ID = "legacy-ssh-task"
	if err := f.s.PutRunnerTask(task); err != nil {
		t.Fatal(err)
	}
	legacy := requestWithHeaders(t, f.h, http.MethodPost, "/api/runners/tasks/claim", map[string]any{
		"availableRuntimes": []string{"mock"},
		"noWait":            true,
	}, auth)
	var legacyResult struct{ Task domain.RunnerTask }
	if err := json.Unmarshal(legacy.Body.Bytes(), &legacyResult); err != nil || legacy.Code != http.StatusOK || legacyResult.Task.ID != task.ID ||
		legacyResult.Task.GitSSHKnownHosts != nil || legacyResult.Task.Invocation == nil || legacyResult.Task.Invocation.GitSSHKeys[gitlabRepo] != gitlabKey {
		t.Fatalf("legacy claim did not preserve the existing SSH payload (HTTP %d)", legacy.Code)
	}

	t.Setenv(gitwork.SSHKnownHostsEnv, "")
	ctx := gitwork.WithSSHKnownHosts(t.Context(), pins)
	ctx = gitwork.WithSSHRemote(gitwork.WithSSHKeys(ctx, result.Task.Invocation.GitSSHKeys), gitlabRepo)
	checkout := t.TempDir()
	if _, err := gitwork.Run(ctx, checkout, "clone", "--", gitlabRepo, "."); err != nil {
		t.Fatalf("clone with the claim's pins and no runner configuration: %v", err)
	}
	if got, want := sshTestGit(t, checkout, "rev-parse", "HEAD"), sshTestGit(t, bare, "--git-dir=.", "rev-parse", "HEAD"); got != want {
		t.Fatalf("cloned HEAD = %s, want %s", got, want)
	}
}

func sshTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func sshTestIdentity(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()
	public, identity, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(identity, "loopforge integration test")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block)), key
}

// A loopback SSH server runs Git on one disposable bare repository, with a
// separate public-key permission for each SSH repository path.
func localSSHGit(t *testing.T, extraKeys map[string]ssh.PublicKey) (remote, privateKey, bare, knownHosts string) {
	t.Helper()
	root := t.TempDir()
	bare = filepath.Join(root, "app.git")
	sshTestGit(t, root, "init", "-b", "main")
	sshTestGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.org", "commit", "--allow-empty", "-m", "base")
	sshTestGit(t, root, "clone", "--bare", ".", bare)
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	host, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, user := sshTestIdentity(t)
	expected := map[string]string{"/team/app.git": ssh.FingerprintSHA256(user)}
	for path, key := range extraKeys {
		if key == nil {
			key = user
		}
		expected[path] = ssh.FingerprintSHA256(key)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	config := &ssh.ServerConfig{PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if conn.User() == "git" {
			fingerprint := ssh.FingerprintSHA256(key)
			for _, allowed := range expected {
				if fingerprint == allowed {
					return &ssh.Permissions{Extensions: map[string]string{"key": fingerprint}}, nil
				}
			}
		}
		return nil, fmt.Errorf("unknown key")
	}}
	config.AddHostKey(host)
	knownHosts = filepath.Join(root, "known_hosts")
	if err := os.WriteFile(knownHosts, []byte(knownhosts.Line([]string{listener.Addr().String()}, host.PublicKey())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	go func(repo string) {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSSHGit(conn, config, repo, expected)
		}
	}(bare)
	return "ssh://git@" + listener.Addr().String() + "/team/app.git", privateKey, bare, knownHosts
}

func serveSSHGit(raw net.Conn, config *ssh.ServerConfig, bare string, expected map[string]string) {
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(30 * time.Second))
	conn, sessions, requests, err := ssh.NewServerConn(raw, config)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(requests)
	for session := range sessions {
		if session.ChannelType() != "session" {
			_ = session.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, commands, err := session.Accept()
		if err != nil {
			return
		}
		defer channel.Close()
		for request := range commands {
			var command struct{ Value string }
			if request.Type != "exec" || ssh.Unmarshal(request.Payload, &command) != nil {
				_ = request.Reply(false, nil)
				continue
			}
			program, path, _ := strings.Cut(command.Value, " ")
			if expected[strings.Trim(path, "'")] != conn.Permissions.Extensions["key"] || (program != "git-upload-pack" && program != "git-receive-pack") {
				_ = request.Reply(false, nil)
				return
			}
			_ = request.Reply(true, nil)
			cmd := exec.Command(program, bare)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = channel, channel, channel.Stderr()
			var exitCode uint32
			if err := cmd.Run(); err != nil {
				exitCode = 1
			}
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{exitCode}))
			// Close like sshd: EOF and close the channel, then let the client
			// hang up. Dropping the TCP connection right away races the
			// client's own channel close ("send disconnect: Broken pipe").
			_ = channel.CloseWrite()
			_ = channel.Close()
			_ = conn.Wait()
			return
		}
	}
}

// GitHub SSH-style addresses keep the GitHub provider (HTTPS + token and
// automatic pull requests); only an explicit provider "ssh" switches transport.
func TestExplicitGitHubSSHAddressKeepsSSHTransport(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	for _, address := range []string{"git@github.com:octo/widgets.git", "ssh://git@github.com/octo/widgets.git", "git clone git@github.com:octo/widgets.git"} {
		resp := cvBRequest(t, f, http.MethodPost, "/api/repositories", map[string]string{"projectId": "platform", "repoUrl": address}, true)
		var repository domain.Repository
		if err := json.Unmarshal(resp.Body.Bytes(), &repository); err != nil || resp.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d, %s", address, resp.Code, resp.Body.String())
		}
		if repository.Provider != domain.RepositoryProviderSSH || repository.GongfengProjectID != "" || !gitwork.IsSSHReference(repository.CloneURL) {
			t.Fatalf("%s saved as provider=%q id=%q clone=%q", address, repository.Provider, repository.GongfengProjectID, repository.CloneURL)
		}
	}
	resp := cvBRequest(t, f, http.MethodPost, "/api/repositories", map[string]string{"projectId": "platform", "provider": domain.RepositoryProviderSSH, "repoUrl": "git@github.com:octo/gadgets.git"}, true)
	var explicit domain.Repository
	if err := json.Unmarshal(resp.Body.Bytes(), &explicit); err != nil || resp.Code != http.StatusOK || explicit.Provider != domain.RepositoryProviderSSH || explicit.CloneURL != "git@github.com:octo/gadgets.git" {
		t.Fatalf("explicit SSH provider: HTTP %d, %s", resp.Code, resp.Body.String())
	}
}
