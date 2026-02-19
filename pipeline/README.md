# pkg/pipeline — 接口定义层 + 核心数据结构

> 重构后此包仅保留：接口定义、Step 数据结构、错误码、模板渲染。不包含任何具体实现。

---

## 保留文件

| 文件 | 内容 | 改动 |
|------|------|------|
| `interfaces.go` | **新建** — 所有子模块的接口契约集中定义 | 新增 |
| `frontmatter.go` | Frontmatter 解析 + `PromptFile` 结构体 | 添加 `StepDefinition` 类型别名 |
| `render.go` | `RenderTemplate` 模板渲染 | 不改 |
| `render_test.go` | 渲染测试 | 不改 |
| `errors.go` | 合并 `error_codes.go` + `error_classify.go` | 合并为单文件 |

## 待删除文件（Phase 7 时执行）

| 文件 | 迁移去向 |
|------|---------|
| `prompt_builder.go` | → `pkg/prompt/assembler.go` |
| `environment.go` | → `pkg/prompt/snapshot.go` + `pkg/prompt/summarizer.go` |
| `context_compressor.go` | → `pkg/memory/compressor.go` |
| `artifacts.go` | → `pkg/memory/tracker.go` |
| `graph_builder.go` | → `pkg/flow/graph.go` |
| `graph_helpers.go` | → `pkg/flow/helpers.go` + `pkg/flow/middleware.go` |

---

## Phase 1 原子任务

### 1.1 创建 `interfaces.go`

定义以下接口（每个接口 ≤ 3 个方法）：

```go
// --- Step 加载 ---
type StepLoader interface {
    Load() ([]*StepDefinition, error)
}

// --- Prompt 构建 ---
type PromptAssembler interface {
    BuildStatic(step *StepDefinition, vars map[string]string) (string, error)
    BuildDynamic(ctx context.Context, step *StepDefinition, vars map[string]string) (string, error)
    HasDynamicContent() bool
}

type ContextSnapshot interface {
    BuildSnapshot(ctx context.Context, currentStepID string, step *StepDefinition) string
}

type InputSummarizer interface {
    Summarize(ctx context.Context, path string) (string, error)
}

// --- Memory ---
type Compressor interface {
    CompressIfNeeded(ctx context.Context, msgs []model.Message, currentTokens int) ([]model.Message, bool, error)
}

type ArtifactTracker interface {
    RecordCompleted(stepID, title, outputPath string) bool
    GetArtifact(stepID string) *ArtifactInfo
    GetAll() map[string]*ArtifactInfo
}

// --- Token ---
type TokenCounter interface {
    Count(ctx context.Context, msgs []model.Message) int
}

type TokenObserver interface {
    OnCompression(beforeTokens, afterTokens int)
}

// --- Flow ---
type FlowBuilder interface {
    Build(steps []*StepDefinition, opts FlowOptions) (*graph.Graph, error)
}

type Middleware interface {
    WrapPreNode(stepID string, step *StepDefinition) graph.BeforeNodeCallback
    WrapPostNode(stepID string, step *StepDefinition) graph.AfterNodeCallback
}

// --- 文件系统 ---
type FileSystem interface {
    fs.ReadFileFS
    Stat(name string) (fs.FileInfo, error)
    ReadDir(name string) ([]fs.DirEntry, error)
}
```

### 1.2 添加 `StepDefinition` 类型别名

在 `frontmatter.go` 中添加：
```go
// StepDefinition is the canonical name for a pipeline step.
// PromptFile is retained as an alias for backward compatibility.
type StepDefinition = PromptFile
```

### 1.3 合并错误码

将 `error_codes.go` + `error_classify.go` 合并为 `errors.go`（纯重命名，逻辑不变）。

---

## 验收标准

- [ ] `interfaces.go` 存在且编译通过
- [ ] 现有代码不受影响（接口与旧代码并行存在）
- [ ] `go build ./...` 通过
- [ ] `go test ./pkg/pipeline/...` 通过
