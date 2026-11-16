package httpapi

import (
	"context"
	"strings"
	"time"

	"ipan/internal/store"
)

// RefreshResult 是令牌保活的执行结果。
type RefreshResult struct {
	Refreshed bool
	Skipped   string
	State     *store.RefreshState
}

// refreshInterval 是两次成功刷新之间的最小间隔。
// 联通云盘的 refresh token 采用滑动过期，2 天刷新一次即可长期保活。
const refreshInterval = 2 * 24 * time.Hour

func refreshStateOK() store.RefreshState {
	now := time.Now().UnixMilli()
	return store.RefreshState{OK: true, AttemptedAt: now, SuccessAt: now}
}

// refreshWithContext 执行一次令牌刷新。
//
// force=false 时，距上次成功刷新不足 refreshInterval 直接跳过；
// 进程内互斥避免后台定时任务与用户请求同时轮换令牌。
func (s *Server) refreshWithContext(ctx context.Context, force bool) (RefreshResult, error) {
	tokens, err := s.store.LoadTokens()
	if err != nil {
		return RefreshResult{}, err
	}
	access := tokens.AccessToken
	if access == "" {
		access = s.cfg.WopanAccessToken
	}
	refresh := tokens.RefreshToken
	if refresh == "" {
		refresh = s.cfg.WopanRefreshToken
	}
	previous, err := s.store.LoadRefreshState()
	if err != nil {
		return RefreshResult{}, err
	}
	if access == "" {
		return RefreshResult{Skipped: "no-access-token", State: previous}, nil
	}
	if refresh == "" {
		return RefreshResult{Skipped: "no-refresh-token", State: previous}, nil
	}
	// 账号已被锁定（8105）时停止一切刷新，等用户在官方客户端解锁后重新登录
	if previous != nil && !previous.OK &&
		(strings.Contains(previous.Error, "8105") || strings.Contains(previous.Error, "账户被锁定")) {
		return RefreshResult{Skipped: "account-locked", State: previous}, nil
	}
	if !force && previous != nil && previous.SuccessAt > 0 &&
		time.Now().UnixMilli()-previous.SuccessAt < int64(refreshInterval/time.Millisecond) {
		return RefreshResult{Skipped: "fresh", State: previous}, nil
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	attemptedAt := time.Now().UnixMilli()
	client := s.client()
	if err := client.RefreshAccessToken(ctx); err != nil {
		// 并发刷新会让当前 refresh token 失效，重读一次确认是否已被别的请求换新
		latest, _ := s.store.LoadTokens()
		if latest.AccessToken != "" && latest.AccessToken != access {
			state := refreshStateOK()
			_ = s.store.SaveRefreshState(state)
			return RefreshResult{Refreshed: true, State: &state}, nil
		}
		state := store.RefreshState{OK: false, AttemptedAt: attemptedAt, Error: err.Error()}
		if previous != nil {
			state.SuccessAt = previous.SuccessAt
		}
		_ = s.store.SaveRefreshState(state)
		return RefreshResult{State: &state}, err
	}

	state := refreshStateOK()
	state.AttemptedAt = attemptedAt
	_ = s.store.SaveRefreshState(state)
	return RefreshResult{Refreshed: true, State: &state}, nil
}

// StartTokenMaintenance 启动后台保活循环，返回停止函数。
//
// 使用进程内定时器定期检查，满足 2 天间隔才真正刷新。
func (s *Server) StartTokenMaintenance(ctx context.Context) func() {
	if s.cfg.TokenCheckInterval <= 0 {
		return func() {}
	}
	interval := time.Duration(s.cfg.TokenCheckInterval) * time.Hour
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		// 启动后稍等片刻再跑第一次，避免和首屏请求抢令牌
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		}
		s.runMaintenanceOnce(ctx)
		for {
			select {
			case <-ticker.C:
				s.runMaintenanceOnce(ctx)
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return func() { close(done) }
}

func (s *Server) runMaintenanceOnce(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	result, err := s.refreshWithContext(ctx, false)
	if err != nil {
		logWarn("令牌保活失败: %v", err)
		return
	}
	if result.Skipped != "" {
		logInfo("令牌保活跳过: %s", result.Skipped)
		return
	}
	if result.Refreshed {
		logInfo("令牌保活成功")
	}
}
