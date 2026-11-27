package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrUpdateReady tells the entry point to exec the newly installed binary.
var ErrUpdateReady = errors.New("runner update ready")

const maxRunnerBinarySize = 512 << 20

// checkRunnerUpdate fetches the platform's runner build and installs it when
// the local binary is older. It is the daemon's self-maintenance path.
func (d *Daemon) checkRunnerUpdate(ctx context.Context) {
	if d.cfg.DisableAutoUpdate {
		return
	}
	binary, err := currentRunnerExecutable()
	if err != nil {
		log.Printf("runner update: locate executable: %v", err)
		return
	}
	if err := d.checkForUpdate(ctx, binary); err != nil && ctx.Err() == nil {
		log.Printf("runner update: %v", err)
	}
}

func currentRunnerExecutable() (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(binary); err == nil {
		return resolved, nil
	}
	return binary, nil
}

var errRunnerUpdateBusy = errors.New("runner is busy")

func (d *Daemon) runnerUpdateEndpoint(targetOS string) string {
	return fmt.Sprintf("%s/runners/agentflow-runner?os=%s&arch=%s", d.client.base, targetOS, runtime.GOARCH)
}

func (d *Daemon) runnerUpdateHashes(ctx context.Context, binary string) (local, expected string, err error) {
	local, err = hashFile(binary)
	if err != nil {
		return "", "", err
	}
	remote, err := runnerBinaryMetadata(ctx, d.runnerUpdateEndpoint(runtime.GOOS))
	return local, remote.SHA256, err
}

func (d *Daemon) checkForUpdate(ctx context.Context, binary string) error {
	_, err := d.installRunnerUpdate(ctx, binary)
	if errors.Is(err, errRunnerUpdateBusy) {
		return nil
	}
	return err
}

// Download and verify outside the update fence. The binary is replaced only
// after the checksum matched.
func (d *Daemon) installRunnerUpdate(ctx context.Context, binary string) (bool, error) {
	if !d.updateDownloadMu.TryLock() {
		return false, errRunnerUpdateBusy
	}
	defer d.updateDownloadMu.Unlock()
	if d.updateReady.Load() {
		return true, nil
	}
	localHash, expectedHash, err := d.runnerUpdateHashes(ctx, binary)
	if err != nil {
		return false, err
	}
	if strings.EqualFold(localHash, expectedHash) {
		return false, nil
	}
	tmp, err := downloadRunnerBinary(ctx, d.runnerUpdateEndpoint(runtime.GOOS), binary, expectedHash)
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp)
	if !d.updateMu.TryLock() {
		return false, errRunnerUpdateBusy
	}
	defer d.updateMu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// A manual upgrade may have happened while the download was in progress.
	if current, err := hashFile(binary); err != nil {
		return false, fmt.Errorf("check runner executable before update: %w", err)
	} else if current != localHash {
		return false, errors.New("runner executable changed during update")
	}
	if err := os.Rename(tmp, binary); err != nil {
		return false, fmt.Errorf("replace runner executable: %w", err)
	}
	d.updateReady.Store(true)
	log.Printf("runner update installed (%s); restarting", expectedHash[:12])
	return true, nil
}

type binaryMetadata struct {
	SHA256 string `json:"sha256"`
}

func runnerBinaryMetadata(ctx context.Context, endpoint string) (binaryMetadata, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"&metadata=1", nil)
	if err != nil {
		return binaryMetadata{}, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(request)
	if err != nil {
		return binaryMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return binaryMetadata{}, fmt.Errorf("runner metadata: HTTP %d", resp.StatusCode)
	}
	var metadata binaryMetadata
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&metadata); err != nil {
		return binaryMetadata{}, fmt.Errorf("decode runner metadata: %w", err)
	}
	if len(metadata.SHA256) != 64 {
		return binaryMetadata{}, errors.New("invalid runner checksum")
	}
	if _, err := hex.DecodeString(metadata.SHA256); err != nil {
		return binaryMetadata{}, fmt.Errorf("invalid runner checksum: %w", err)
	}
	return metadata, nil
}

func downloadRunnerBinary(ctx context.Context, endpoint, binary, expectedHash string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download runner: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(binary), ".agentflow-runner-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	remove := true
	defer func() {
		if remove {
			os.Remove(tmp.Name())
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(resp.Body, maxRunnerBinarySize+1))
	if err != nil {
		return "", err
	}
	if n == 0 || n > maxRunnerBinarySize {
		return "", errors.New("runner binary is empty or too large")
	}
	if got := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(got, expectedHash) {
		return "", fmt.Errorf("runner checksum mismatch: expected %s, got %s", expectedHash, got)
	}
	if err := tmp.Chmod(0o755); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	remove = false
	return tmp.Name(), nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
