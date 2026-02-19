# pkg/prompt — Prompt 构建

> 从 `prompt_builder.go` 和 `environment.go` 抽出，提供三层 Prompt 组装、运行时上下文快照和输入文件摘要能力。

---

## 来源文件映射

| 当前位置 | 迁移到本包 |
|---------|-----------|
| `pkg/pipeline/prompt_builder.go` (182 行) | `assembler.go` + `markers.go` |
| `pkg/pipeline/environment.go` → `BuildSnapshot` 及 4 个子方法 (行 52-261) | `snapshot.go` |
| `pkg/pipeline/environment.go` → `summarizeFile` / `llmSummarize` / `fallbackSummary` (行 147-228) | `summarizer.go` |

---

## Phase 4 原子任务

### 4.1 创建 `assembler.go` — Assembler

```go
package prompt

// Assembler 实现 pipeline.PromptAssembler 接口
type Assembler struct {
    corePrompt     string
    toolsReference string
    snapshot       pipeline.ContextSnapshot  // ← 接口，非具体 *EnvironmentBuilder
}

func NewAssembler(
    corePromptPath string,
    toolsRefPath string,
    fs pipeline.FileSystem,       // ← 通过接口读文件
    snapshot pipeline.ContextSnapshot,  // ← 接口注入
) *Assembler

func (a *Assembler) BuildStatic(step *pipeline.StepDefinition, vars map[string]string) (string, error)
func (a *Assembler) BuildDynamic(ctx context.Context, step *pipeline.StepDefinition, vars map[string]string) (string, error)
func (a *Assembler) HasDynamicContent() bool
```

**与当前 `PromptBuilder` 的区别**：
1. **`snapshot` 字段类型从 `*EnvironmentBuilder` → `pipeline.ContextSnapshot` 接口**
2. **读文件通过 `pipeline.FileSystem` 接口**，不直接 `os.ReadFile`
3. **不再有 `MakePreNodeCallback`** — 移到 `pkg/flow/middleware.go`
4. `BuildStatic` 对应当前 `BuildStaticInstruction` (行 41-75)
5. `BuildDynamic` 对应当前 `buildFullInstruction` (行 106-146)

---

### 4.2 创建 `snapshot.go` — Snapshot

```go
package prompt

// Snapshot 实现 pipeline.ContextSnapshot 接口
type Snapshot struct {
    steps      []*pipeline.StepDefinition
    tracker    pipeline.ArtifactTracker    // ← 接口
    summarizer pipeline.InputSummarizer    // ← 接口
    toolNames  func(string) []string       // ← 函数注入，不持有 tool.ToolSet
}

func NewSnapshot(
    steps []*pipeline.StepDefinition,
    tracker pipeline.ArtifactTracker,
    summarizer pipeline.InputSummarizer,
    toolNames func(string) []string,
) *Snapshot

func (s *Snapshot) BuildSnapshot(ctx context.Context, currentStepID string, step *pipeline.StepDefinition) string
```

**内部方法**（私有，从 `environment.go` 搬移）：
- `buildProgress(currentStepID)` → 来源 `environment.go:66-94`
- `buildInputSummaries(ctx, step)` → 来源 `environment.go:97-125`
  - 改动：调用 `s.summarizer.Summarize()` 而非自己读文件+调LLM
- `buildAvailableTools(step)` → 来源 `environment.go:231-261`
  - 改动：调用 `s.toolNames(stepID)` 获取工具名列表，不再遍历 `tool.ToolSet`
- `buildOutputContract(step)` → 来源 `environment.go:264-285`
  - 无改动

**与当前 `EnvironmentBuilder` 的区别**：
1. **不再持有 `model.Model`** — 摘要交给 `InputSummarizer` 接口
2. **不再持有 `map[string]tool.ToolSet`** — 通过 `toolNames` 函数获取名称
3. **不再持有 `baseDir`** — 文件访问全部通过接口
4. **不再有 `MakePreNodeCallback`** — 移到 `pkg/flow/middleware.go`
5. **不再有 `summarizeFile` / `llmSummarize`** — 移到 `summarizer.go`

---

### 4.3 Snapshot 依赖注入

`Snapshot` 的 3 个依赖全部是接口：

| 依赖 | 接口 | 提供者 |
|------|------|--------|
| `tracker` | `pipeline.ArtifactTracker` | `memory.FileTracker` |
| `summarizer` | `pipeline.InputSummarizer` | `prompt.LLMSummarizer` 或 `prompt.FallbackSummarizer` |
| `toolNames` | `func(string) []string` | 编排层构造时传入 |

`toolNames` 函数的构造示例（在编排层）：
```go
toolNames := func(stepID string) []string {
    for _, step := range steps {
        if step.Frontmatter.Step == stepID {
            return step.Frontmatter.EffectiveTools()
        }
    }
    return nil
}
```

---

### 4.4 创建 `summarizer.go` — LLMSummarizer + FallbackSummarizer

```go
package prompt

// LLMSummarizer 实现 pipeline.InputSummarizer 接口
type LLMSummarizer struct {
    model model.Model
    fs    pipeline.FileSystem  // ← 接口读文件
    cache sync.Map
}

func NewLLMSummarizer(model model.Model, fs pipeline.FileSystem) *LLMSummarizer
func (s *LLMSummarizer) Summarize(ctx context.Context, path string) (string, error)
// 内部逻辑：读文件 → 截断4000字 → 调LLM生成2-3行摘要 → 缓存

// FallbackSummarizer 无 LLM 时的降级实现
type FallbackSummarizer struct {
    fs pipeline.FileSystem
}

func NewFallbackSummarizer(fs pipeline.FileSystem) *FallbackSummarizer
func (s *FallbackSummarizer) Summarize(ctx context.Context, path string) (string, error)
// 内部逻辑：读文件 → 返回前5行
```

**来源代码**：
- `LLMSummarizer.Summarize` → `environment.go:147-173` (`summarizeFile`) + `environment.go:176-219` (`llmSummarize`)
- `FallbackSummarizer.Summarize` → `environment.go:222-228` (`fallbackSummary`)

---

### 4.5 FallbackSummarizer — 无 LLM 降级

已包含在 4.4 中。当 `model.Model` 为 nil 时，编排层选择 `FallbackSummarizer` 而非 `LLMSummarizer`。

---

### 4.6 创建 `markers.go` — Layer 标记工具函数

```go
package prompt

func FormatLayerMarker() string           // → "<system_core_prompt>"
func IsProtectedSystemMessage(content string) bool  // 检查是否含 Layer1/2 标记
```

**来源**：`prompt_builder.go:156-165`，原样搬移。

---

### 4.7 创建 `assembler_test.go`

测试用例：
- `TestAssembler_BuildStatic` — 无 Snapshot 时仅输出 Layer1 + Layer2 body
- `TestAssembler_BuildDynamic` — 有 Snapshot 时输出完整 Layer1 + 动态 Layer2 + body
- `TestAssembler_HasDynamicContent` — snapshot 非 nil 返回 true
- `TestAssembler_TemplateVars` — `{{stage}}` / `{{output_path}}` 正确替换

Mock 依赖：`pipeline.ContextSnapshot`（返回固定 XML 字符串）

---

### 4.8 创建 `snapshot_test.go`

测试用例：
- `TestSnapshot_BuildProgress` — 正确显示 ✅/🔄/⬚ 状态
- `TestSnapshot_BuildInputSummaries` — 调用 Summarizer 接口，非直接读文件
- `TestSnapshot_BuildAvailableTools` — 通过 toolNames 函数获取工具名
- `TestSnapshot_BuildOutputContract` — 正确渲染 next/fallback

Mock 依赖：`pipeline.ArtifactTracker`（返回预设数据）+ `pipeline.InputSummarizer`（返回固定摘要）

---

## 依赖关系

```
pkg/prompt/
  assembler.go   → pipeline.ContextSnapshot (接口)
                 → pipeline.FileSystem (接口)
                 → pipeline.StepDefinition (数据结构)
  snapshot.go    → pipeline.ArtifactTracker (接口)
                 → pipeline.InputSummarizer (接口)
  summarizer.go  → pipeline.FileSystem (接口)
                 → model.Model (框架类型)
  markers.go     → 无外部依赖
  
  依赖: pkg/pipeline (仅接口)
  不依赖: pkg/memory, pkg/token, pkg/flow, pkg/step
```

## 验收标准

- [ ] `Assembler` 编译通过，实现 `pipeline.PromptAssembler`
- [ ] `Snapshot` 编译通过，实现 `pipeline.ContextSnapshot`
- [ ] `LLMSummarizer` 编译通过，实现 `pipeline.InputSummarizer`
- [ ] 不包含任何 `os.ReadFile` / `os.Stat` 直接调用
- [ ] 不包含任何 `tool.ToolSet` 直接引用
- [ ] 不包含任何 `MakePreNodeCallback` 方法
- [ ] `go test ./pkg/prompt/...` 全部通过
