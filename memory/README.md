# pkg/memory — 上下文压缩 + 产出物追踪

> 从 `context_compressor.go` 和 `artifacts.go` 抽出，提供消息历史压缩和步骤产出物追踪能力。

---

## 来源文件映射

| 当前位置 | 迁移到本包 |
|---------|-----------|
| `pkg/pipeline/context_compressor.go` (237 行) | `compressor.go` |
| `pkg/pipeline/context_compressor.go` → 摘要格式化 | 已在 `prompt_builder.go` 中，移入 `summary.go` |
| `pkg/pipeline/artifacts.go` (118 行) | `tracker.go` + `types.go` |

---

## Phase 3 原子任务

### 3.1 创建 `compressor.go` — LLMCompressor

```go
package memory

// LLMCompressor 实现 pipeline.Compressor 接口
type LLMCompressor struct {
    model           model.Model
    counter         pipeline.TokenCounter   // ← 注入接口，不自己算 token
    contextWindow   int
    threshold       float64
    keepRecentTurns int
}

func NewLLMCompressor(
    model model.Model,
    counter pipeline.TokenCounter,  // ← 依赖接口
    contextWindow int,
    threshold float64,
    keepRecentTurns int,
) *LLMCompressor

func (c *LLMCompressor) CompressIfNeeded(
    ctx context.Context,
    msgs []model.Message,
    currentTokens int,
) ([]model.Message, bool, error)
```

**与当前 `ContextCompressor` 的区别**：
1. **不再持有 `tokenCounter`** — 通过 `pipeline.TokenCounter` 接口注入
2. **不再有 `MakePreNodeCallback`** — 回调生成移到 `pkg/flow/middleware.go`
3. **不再有 `SetObserver`** — Observer 模式移到中间件层
4. 内部 `compress()` 和 `callSummarize()` 方法逻辑保持不变

**来源代码位置**：
- `CompressIfNeeded` → `context_compressor.go:70-93`
- `compress` → `context_compressor.go:98-155`
- `callSummarize` → `context_compressor.go:158-191`

---

### 3.2 创建 `summary.go` — 摘要消息格式化

从 `prompt_builder.go:167-181` 提取：

```go
package memory

const SummaryPrefix = "[上下文摘要 — 以下是之前对话的压缩总结]\n"

func IsSummaryMessage(content string) bool
func FormatSummaryMessage(summary string) model.Message
```

逻辑完全不变，仅搬移位置。

---

### 3.3 创建 `tracker.go` — FileTracker

```go
package memory

// FileTracker 实现 pipeline.ArtifactTracker 接口
type FileTracker struct {
    fs   pipeline.FileSystem   // ← 注入接口，不直接 os.Stat
    mu   sync.RWMutex
    data map[string]*ArtifactInfo
}

func NewFileTracker(fs pipeline.FileSystem) *FileTracker

func (t *FileTracker) RecordCompleted(stepID, title, outputPath string) bool
func (t *FileTracker) GetArtifact(stepID string) *ArtifactInfo
func (t *FileTracker) GetAll() map[string]*ArtifactInfo
```

**与当前 `ArtifactTracker` 的区别**：
1. **不再持有 `baseDir string`** — 通过 `FileSystem` 接口访问文件
2. **不再有 `MakePostNodeCallback`** — 移到 `pkg/flow/middleware.go`
3. `countLines` 辅助函数保留在本文件中

**来源代码位置**：
- `RecordIfCompleted` → `artifacts.go:43-64`
- `GetArtifacts` → `artifacts.go:67-76`
- `GetArtifact` → `artifacts.go:79-87`
- `countLines` → `artifacts.go:102-117`

---

### 3.4 创建 `types.go` — ArtifactInfo 数据结构

```go
package memory

type ArtifactInfo struct {
    StepID    string
    Title     string
    FilePath  string
    Status    string    // "completed" | "in_progress" | "pending"
    Summary   string
    LineCount int
    CreatedAt time.Time
}
```

来源：`artifacts.go:16-24`，原样搬移。

---

### 3.5 回调方法移除

以下方法 **不在本包实现**，移到 `pkg/flow/middleware.go`：

| 当前方法 | 新位置 |
|---------|--------|
| `ContextCompressor.MakePreNodeCallback()` | `flow.CompressionMiddleware.WrapPreNode()` |
| `ContextCompressor.SetObserver()` | `flow.CompressionMiddleware` 构造时注入 |
| `ArtifactTracker.MakePostNodeCallback()` | `flow.ArtifactRecordMiddleware.WrapPostNode()` |

---

### 3.6 创建 `compressor_test.go`

测试用例：
- `TestLLMCompressor_BelowThreshold` — 未达阈值，不压缩
- `TestLLMCompressor_AboveThreshold` — 达到阈值，触发压缩
- `TestLLMCompressor_TooFewMessages` — 消息少于 keepRecentTurns*2，不压缩
- `TestLLMCompressor_SummarizeFailure` — LLM 调用失败，返回原始消息

Mock 依赖：`pipeline.TokenCounter`（返回固定值）+ `model.Model`（返回固定摘要）

---

### 3.7 创建 `tracker_test.go`

测试用例：
- `TestFileTracker_RecordCompleted` — 文件存在时记录成功
- `TestFileTracker_FileNotFound` — 文件不存在返回 false
- `TestFileTracker_GetAll` — 返回所有已记录的 artifacts 的快照（非引用）
- `TestFileTracker_GetArtifact` — 按 stepID 获取单个 artifact

Mock 依赖：`pipeline.FileSystem`（内存实现，`testing/fstest.MapFS`）

---

## 依赖关系

```
pkg/memory/
  compressor.go → pipeline.TokenCounter (接口)
                → pipeline.Compressor (实现此接口)
                → model.Model (框架类型)
  tracker.go    → pipeline.FileSystem (接口)
                → pipeline.ArtifactTracker (实现此接口)
  summary.go    → model.Message (框架类型)
  
  依赖: pkg/pipeline (仅接口), pkg/token (无 — 通过接口解耦)
```

## 验收标准

- [ ] `pkg/memory/compressor.go` 编译通过，实现 `pipeline.Compressor`
- [ ] `pkg/memory/tracker.go` 编译通过，实现 `pipeline.ArtifactTracker`
- [ ] 不包含任何 `MakePreNodeCallback` / `MakePostNodeCallback` 方法
- [ ] 不包含任何 `os.Stat` / `os.ReadFile` 直接调用
- [ ] `go test ./pkg/memory/...` 全部通过
- [ ] `go build ./...` 全项目编译通过
