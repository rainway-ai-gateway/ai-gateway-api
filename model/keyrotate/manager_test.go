// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License. All rights reserved.

package keyrotate

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/hkdf"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xcrypto"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
)

// fakeStorager records rewrites and lets tests script Acquire responses.
type fakeStorager struct {
	mu         sync.Mutex
	acquireErr error
	rows       map[string][]RowRef
	listCalls  map[string]int
	rewrites   []rewriteRec
	finish     *finishRec
	progress   [][3]interface{}
	listTasks  func(ctx context.Context, filter *TaskFilter) ([]*SweepTask, int64, error)
}

type rewriteRec struct {
	Table string
	PK    interface{}
	Value string
}

type finishRec struct {
	Status  Status
	Cause   string
	Summary map[string]*TableCount
}

func (f *fakeStorager) AcquireTask(ctx context.Context, task *SweepTask) error {
	return f.acquireErr
}
func (f *fakeStorager) GetTask(ctx context.Context, taskID string) (*SweepTask, error) {
	return nil, nil
}
func (f *fakeStorager) ListTasks(ctx context.Context, filter *TaskFilter) ([]*SweepTask, int64, error) {
	if f.listTasks != nil {
		return f.listTasks(ctx, filter)
	}
	return nil, 0, nil
}
func (f *fakeStorager) AddProgress(ctx context.Context, dbCtx lib.DBContexter, taskID string, scanned, rewritten int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progress = append(f.progress, [3]interface{}{taskID, scanned, rewritten})
	return nil
}
func (f *fakeStorager) FinishTask(ctx context.Context, taskID string, status Status, cause string,
	summary map[string]*TableCount, duration time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finish = &finishRec{Status: status, Cause: cause, Summary: summary}
	return nil
}
func (f *fakeStorager) ListRows(ctx context.Context, t SweepTable, cursor interface{}, limit int) ([]RowRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listCalls == nil {
		f.listCalls = map[string]int{}
	}
	f.listCalls[t.Name]++
	if f.listCalls[t.Name] == 1 {
		return f.rows[t.Name], nil
	}
	return nil, nil
}
func (f *fakeStorager) RewriteRow(ctx context.Context, dbCtx lib.DBContexter, t SweepTable, pk interface{}, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rewrites = append(f.rewrites, rewriteRec{Table: t.Name, PK: pk, Value: value})
	return nil
}

// setupRing installs a temp keyring and loads it into the global stateful ring.
func setupRing(t *testing.T, active int, seeds ...byte) *xcrypto.Keyring {
	t.Helper()
	content := "ActiveKeyID = " + strconv.Itoa(active) + "\n[Keys]\n"
	for i, seed := range seeds {
		key := make([]byte, 32)
		for j := range key {
			key[j] = seed + byte(j)
		}
		content += "  " + strconv.Itoa(i+1) + " = \"" + base64.StdEncoding.EncodeToString(key) + "\"\n"
	}
	path := filepath.Join(t.TempDir(), "master.keys")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	prev := stateful.DefaultConfig
	prevRing := stateful.SecretRing()
	stateful.DefaultConfig = &stateful.Config{}
	stateful.DefaultConfig.Security.MasterKeyFile = path
	require.NoError(t, stateful.LoadSecretRing())
	t.Cleanup(func() {
		stateful.DefaultConfig = prev
		if prevRing == nil {
			stateful.DefaultConfig = &stateful.Config{}
		}
		_ = stateful.LoadSecretRing()
	})
	return stateful.SecretRing()
}

// encKeyFor derives the db-enc-key HKDF output for a raw master key, matching
// xcrypto's derivation, so tests can craft ciphertext under any keyID.
func encKeyFor(t *testing.T, master []byte) []byte {
	t.Helper()
	r := hkdf.New(sha256.New, master, nil, []byte("db-enc-key"))
	out := make([]byte, 32)
	_, err := io.ReadFull(r, out)
	require.NoError(t, err)
	return out
}

func TestTriggerParamValidation(t *testing.T) {
	m := NewManager(&fakeStorager{}, nil)
	_, err := m.Trigger(context.Background(), "bad", false, "all", "")
	require.Error(t, err)
	_, err = m.Trigger(context.Background(), ModeReencrypt, false, "bad", "")
	require.Error(t, err)

	fs := &fakeStorager{}
	m = NewManager(fs, nil)
	task, err := m.Trigger(context.Background(), "", false, "", "admin")
	require.NoError(t, err)
	assert.Equal(t, ModeReencrypt, task.Mode)
	assert.Equal(t, ScopeAll, task.Scope)
	assert.Equal(t, "admin", task.CreatedBy)

	require.Eventually(t, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return fs.finish != nil
	}, 3*time.Second, 10*time.Millisecond)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	assert.Equal(t, StatusSucceeded, fs.finish.Status)
}

func TestTriggerConflict(t *testing.T) {
	fs := &fakeStorager{acquireErr: &ErrConflict{HolderTaskID: "rsp-existing"}}
	m := NewManager(fs, nil)
	task, err := m.Trigger(context.Background(), ModeReencrypt, false, ScopeAll, "")
	require.Error(t, err)
	var conflict *ErrConflict
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, "rsp-existing", task.TaskID)
}

func TestRunReencryptConvergesRows(t *testing.T) {
	kr := setupRing(t, 2, 1, 100)
	require.NotNil(t, kr)

	// craft a ciphertext under keyID=1 (rotation window artifact)
	master1 := make([]byte, 32)
	for i := range master1 {
		master1[i] = 1 + byte(i)
	}
	oldCT1, err := xcrypto.Encrypt("sk-old1", encKeyFor(t, master1), 1)
	require.NoError(t, err)

	newCT, err := kr.Encrypt("sk-new")
	require.NoError(t, err)

	fs := &fakeStorager{rows: map[string][]RowRef{
		"api_keys": {
			{PK: int64(1), Value: "plain-legacy"},
			{PK: int64(2), Value: oldCT1},
			{PK: int64(3), Value: newCT},
		},
	}}
	m := NewManager(fs, nil)
	m.run(&SweepTask{TaskID: "t1", Mode: ModeReencrypt, Scope: ScopeAPIKeys, ActiveKeyID: 2})

	fs.mu.Lock()
	defer fs.mu.Unlock()
	require.NotNil(t, fs.finish)
	assert.Equal(t, StatusSucceeded, fs.finish.Status)

	// plaintext row and old-keyID row rewritten; current-keyID row skipped
	require.Len(t, fs.rewrites, 2)
	byPK := map[interface{}]string{}
	for _, r := range fs.rewrites {
		byPK[r.PK] = r.Value
	}
	pt, err := kr.Decrypt(byPK[int64(1)])
	require.NoError(t, err)
	assert.Equal(t, "plain-legacy", pt)
	id, _ := xcrypto.KeyIDOf(byPK[int64(2)])
	assert.Equal(t, uint8(2), id, "old-keyID row must be re-encrypted under active key")

	sum := fs.finish.Summary["api_keys"]
	require.NotNil(t, sum)
	assert.Equal(t, int64(2), sum.Rewritten)
	assert.Equal(t, int64(1), sum.Skipped)
}

func TestRunDecryptMode(t *testing.T) {
	kr := setupRing(t, 1, 7)
	ct, err := kr.Encrypt("sk-secret")
	require.NoError(t, err)

	fs := &fakeStorager{rows: map[string][]RowRef{
		"providers": {{PK: "p1", Value: ct}, {PK: "p2", Value: "already-plain"}},
	}}
	m := NewManager(fs, nil)
	m.run(&SweepTask{TaskID: "t2", Mode: ModeDecrypt, Scope: ScopeProviders})

	fs.mu.Lock()
	defer fs.mu.Unlock()
	require.NotNil(t, fs.finish)
	assert.Equal(t, StatusSucceeded, fs.finish.Status)
	require.Len(t, fs.rewrites, 1)
	assert.Equal(t, "sk-secret", fs.rewrites[0].Value)
	assert.False(t, xcrypto.IsEncrypted(fs.rewrites[0].Value))

	sum := fs.finish.Summary["providers"]
	require.NotNil(t, sum)
	assert.Equal(t, int64(1), sum.Rewritten)
	assert.Equal(t, int64(1), sum.Skipped)
}

func TestDryRunDoesNotRewrite(t *testing.T) {
	kr := setupRing(t, 1, 9)
	ct, _ := kr.Encrypt("sk-x")

	fs := &fakeStorager{rows: map[string][]RowRef{
		"api_keys": {{PK: int64(1), Value: "plain"}, {PK: int64(2), Value: ct}},
	}}
	m := NewManager(fs, nil)
	m.run(&SweepTask{TaskID: "t3", Mode: ModeReencrypt, DryRun: true, Scope: ScopeAPIKeys})

	fs.mu.Lock()
	defer fs.mu.Unlock()
	assert.Empty(t, fs.rewrites)
	require.NotNil(t, fs.finish)
	assert.Equal(t, StatusSucceeded, fs.finish.Status)
	sum := fs.finish.Summary["api_keys"]
	assert.Equal(t, int64(1), sum.Rewritten, "dry_run counts the plaintext row as would-be rewrite")
	assert.Equal(t, int64(1), sum.Skipped, "active-key ciphertext row is already converged")
}

func TestDecryptFailureFailsTask(t *testing.T) {
	setupRing(t, 1, 3)
	fs := &fakeStorager{rows: map[string][]RowRef{
		"api_keys": {{PK: int64(1), Value: "enc$v1$" + base64.StdEncoding.EncodeToString([]byte{9, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})}},
	}}
	m := NewManager(fs, nil)
	m.run(&SweepTask{TaskID: "t4", Mode: ModeReencrypt, Scope: ScopeAPIKeys})

	fs.mu.Lock()
	defer fs.mu.Unlock()
	require.NotNil(t, fs.finish)
	assert.Equal(t, StatusFailed, fs.finish.Status)
	assert.Contains(t, fs.finish.Cause, "decrypt fail")
}

func TestListTasksDelegates(t *testing.T) {
	fs := &fakeStorager{
		listTasks: func(ctx context.Context, filter *TaskFilter) ([]*SweepTask, int64, error) {
			return []*SweepTask{{TaskID: "rsp-1", Status: StatusSucceeded, CreatedBy: "admin"}}, 1, nil
		},
	}
	m := NewManager(fs, nil)

	tasks, total, err := m.ListTasks(context.Background(), &TaskFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, tasks, 1)
	assert.Equal(t, "rsp-1", tasks[0].TaskID)
	assert.Equal(t, "admin", tasks[0].CreatedBy)
}
