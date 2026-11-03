//go:build mysql

package ai_cache_update_test

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAICacheRules_ReplaceConflict409MySQL（AC-1-019）验证同一集合被另一请求
// 整体替换占锁时，PUT 返回 409 Conflict 而不是 500，且文案不含驱动错误原文。
// 用真实命名锁占位（GET_LOCK），因此该路径在 MySQL 下必现（issue #295）。
func TestAICacheRules_ReplaceConflict409MySQL(t *testing.T) {
	adminDSN := os.Getenv("AIAPI_MYSQL_DSN")
	if adminDSN == "" {
		t.Skip("AIAPI_MYSQL_DSN not set, skipping MySQL concurrency test")
	}

	sm, err := testutil.StartServerWithMySQL(adminDSN)
	require.NoError(t, err, "start MySQL-backed server")
	defer sm.Shutdown()

	client := &testutil.Client{BaseURL: sm.ServerURL, HTTPClient: &http.Client{Timeout: 60 * time.Second}}

	db := testutil.OpenDB(t, sm)
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	var got sql.NullInt64
	require.NoError(t, conn.QueryRowContext(context.Background(),
		"SELECT GET_LOCK(?, ?)", "ai_gateway:replace:ai_cache_rules", 0).Scan(&got))
	require.True(t, got.Valid && got.Int64 == 1, "test must hold the replace lock")
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", "ai_gateway:replace:ai_cache_rules")

	resp, err := client.Put(aiCacheRulesPath, map[string]interface{}{
		"rules": []interface{}{map[string]interface{}{"name": "conflict-409", "cond": "default_t()"}},
	})
	require.NoError(t, err)

	assert.Equal(t, 409, resp.ErrNum)
	assert.Contains(t, resp.ErrMsg, "Conflict")
	assert.False(t, testutil.LeaksDriverError(resp.ErrMsg), "ErrMsg must not leak driver details: %s", resp.ErrMsg)

	// 释放锁后同一请求成功，集合可用。
	_, err = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", "ai_gateway:replace:ai_cache_rules")
	require.NoError(t, err)

	resp, err = client.Put(aiCacheRulesPath, map[string]interface{}{
		"rules": []interface{}{map[string]interface{}{"name": "conflict-409", "cond": "default_t()"}},
	})
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum, resp.ErrMsg)
}
