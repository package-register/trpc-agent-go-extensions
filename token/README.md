# pkg/token — Token 计量与监控

> 从 `context_compressor.go:countTokens` 和 `internal/api/agui/token_monitor.go` 抽出，提供独立的 token 计数与累计监控能力。

---

## 来源文件映射

| 当前位置 | 迁移到本包 |
|---------|-----------|
| `pkg/pipeline/context_compressor.go` → `countTokens()` 方法 (行 194-203) | `counter.go` |
| `internal/api/agui/token_monitor.go` (完整文件 181 行) | `monitor.go` |

---

## Phase 2 原子任务

### 2.1 创建 `counter.go` — SimpleCounter

```go
package token

// SimpleCounter 实现 pipeline.TokenCounter 接口
type SimpleCounter struct {
    inner model.TokenCounter  // 框架提供的 CountTokensRange
}

func NewSimpleCounter() *SimpleCounter

func (c *SimpleCounter) Count(ctx context.Context, msgs []model.Message) int
// 逻辑：调用 inner.CountTokensRange，失败时 fallback 到 len(content)/4
```

**来源代码**：
```go
// 当前在 context_compressor.go:194-203
func (c *ContextCompressor) countTokens(ctx context.Context, msgs []model.Message) int {
    total, err := c.tokenCounter.CountTokensRange(ctx, msgs, 0, len(msgs))
    if err != nil {
        for _, m := range msgs {
            total += len(m.Content) / 4
        }
    }
    return total
}
```

**改动**：提取为独立结构体，实现 `pipeline.TokenCounter` 接口。

---

### 2.2 创建 `monitor.go` — TokenMonitor

将 `internal/api/agui/token_monitor.go` 完整移入本包。

**改动清单**：
- 包名 `agui` → `token`
- 实现 `pipeline.TokenObserver` 接口（`OnCompression` 方法已存在）
- `TokenUsage` 结构体、`NewTokenMonitor`、`RecordUsage`、`ProcessEvent`、`GetStats`、`IsWarning`、`IsCritical`、`DrainPendingUpdate`、`Reset` — 全部搬移，逻辑不变

**注意**：`ProcessEvent` 依赖 `trpc.group/trpc-go/trpc-agent-go/event`，此依赖保留。

---

### 2.3 创建 `counter_test.go`

测试用例：
- `TestSimpleCounter_Normal` — 正常计数
- `TestSimpleCounter_Fallback` — inner 返回错误时降级到 len/4
- `TestSimpleCounter_EmptyMessages` — 空消息列表返回 0

---

### 2.4 创建 `monitor_test.go`

测试用例：
- `TestMonitor_RecordUsage` — 记录后 GetStats 正确
- `TestMonitor_OnCompression` — 压缩后 totalTokens 减少
- `TestMonitor_DrainPendingUpdate` — 压缩后 pending=true，drain 后 pending=false
- `TestMonitor_IsWarning` — 超过阈值返回 true
- `TestMonitor_HistoryLimit` — 超过 1000 条自动截断

---

### 2.5 更新 import 路径

`internal/api/agui/` 中引用 `TokenMonitor` 的文件：
- `agent.go` — `NewTokenMonitor` 创建
- `translator.go` — `ProcessEvent` 和 `DrainPendingUpdate` 调用
- `server.go` — `NewTokenMonitor` 创建
- `handler.go` — 可能引用

全部改为 `import "web-plugin/pkg/token"`，删除旧的 `token_monitor.go`。

---

## 依赖关系

```
pkg/token/
  counter.go  → model.TokenCounter (框架类型)
  monitor.go  → event.Event (框架类型)
  
  不依赖任何 pkg/ 内的其他子模块 ← 这是最底层的模块
```

## 验收标准

- [ ] `pkg/token/counter.go` 编译通过，实现 `pipeline.TokenCounter`
- [ ] `pkg/token/monitor.go` 编译通过，实现 `pipeline.TokenObserver`
- [ ] `go test ./pkg/token/...` 全部通过
- [ ] `internal/api/agui/token_monitor.go` 已删除
- [ ] `go build ./...` 全项目编译通过
