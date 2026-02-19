# pkg/flow — 流程构建 + 可组合中间件

> 从 `graph_builder.go` 和 `graph_helpers.go` 抽出，提供 Chain / Agent / Graph 三种流程构建器和可组合的中间件系统。

---

## 来源文件映射

| 当前位置 | 迁移到本包 |
|---------|-----------|
| `pkg/pipeline/graph_builder.go` → `BuildGraphFromPrompts()` (行 99-318) | `graph.go` |
| `pkg/pipeline/graph_builder.go` → `BuildOptions` (行 23-36) | `types.go` |
| `pkg/pipeline/graph_helpers.go` → `resolveToolSets` / `makeFallbackRouter` / `makeConfirmNode` / `wrapToolsNode` (行 44-140) | `helpers.go` |
| `pkg/pipeline/graph_helpers.go` → `clearPipelineErrorCode` (行 107-116) | `helpers.go` |
| `pkg/pipeline/graph_builder.go` → 65 行 PreNodeCallback 闭包 (行 161-226) | `middleware.go` (拆解为独立中间件) |
| `pkg/pipeline/context_compressor.go` → `MakePreNodeCallback` (行 208-236) | `middleware.go` → `CompressionMiddleware` |
| `pkg/pipeline/prompt_builder.go` → `MakePreNodeCallback` (行 80-103) | `middleware.go` → `PromptInjectionMiddleware` |
| `pkg/pipeline/artifacts.go` → `MakePostNodeCallback` (行 91-99) | `middleware.go` → `ArtifactRecordMiddleware` |

---

## Phase 6 原子任务

### 6.1 创建 `types.go` — FlowOptions

```go
package flow

import (
    "web-plugin/pkg/pipeline"
    "trpc.group/trpc-go/trpc-agent-go/model"
    "trpc.group/trpc-go/trpc-agent-go/tool"
)

// FlowOptions 替代 BuildOptions，精简为通用字段
type FlowOptions struct {
    Model            model.Model
    ToolSets         map[string]tool.ToolSet
    AllowMissing     bool
    MaxOutputTokens  int
    Middlewares       []pipeline.Middleware  // ← 可组合中间件列表
}
```

**与当前 `BuildOptions` 的区别**：
- **移除** `PromptsDir`, `TemplatesDir`, `BaseVars`, `SystemInstruction` — 这些已由 `PromptAssembler` 内部处理
- **移除** `ArtifactTracker`, `PromptBuilder`, `EnvironmentBuilder`, `ContextCompressor` — 全部变为 Middleware
- **新增** `Middlewares` — 可组合的中间件列表

---

### 6.2 创建 `middleware.go` — MiddlewareChain

```go
package flow

// MiddlewareChain 组合多个 Middleware 为一个
type MiddlewareChain struct {
    items []pipeline.Middleware
}

func NewMiddlewareChain(items ...pipeline.Middleware) *MiddlewareChain

// WrapPreNode 按顺序执行所有中间件的 PreNode 回调，合并 State
func (c *MiddlewareChain) WrapPreNode(stepID string, step *pipeline.StepDefinition) graph.BeforeNodeCallback

// WrapPostNode 按顺序执行所有中间件的 PostNode 回调
func (c *MiddlewareChain) WrapPostNode(stepID string, step *pipeline.StepDefinition) graph.AfterNodeCallback
```

**这是当前 65 行 mega-closure 的替代品**。原来：
```go
// graph_builder.go:161-226 — 一个闭包做所有事
nodeOpts = append(nodeOpts, graph.WithPreNodeCallback(
    func(ctx, cbCtx, state) {
        // compressor...
        // promptBuilder...
        // envBuilder (fallback)...
    }
))
```

变成：
```go
chain := flow.NewMiddlewareChain(
    flow.NewCompressionMiddleware(compressor, counter, observer),
    flow.NewPromptInjectionMiddleware(assembler),
)
// chain.WrapPreNode(stepID, step) 返回一个干净的 BeforeNodeCallback
```

---

### 6.3 创建 `CompressionMiddleware`

```go
package flow

// CompressionMiddleware 实现 pipeline.Middleware
// 在 PreNode 时检查 token 使用率，触发压缩
type CompressionMiddleware struct {
    compressor pipeline.Compressor     // ← 接口
    counter    pipeline.TokenCounter   // ← 接口
    observer   pipeline.TokenObserver  // ← 接口，可为 nil
}

func NewCompressionMiddleware(
    compressor pipeline.Compressor,
    counter pipeline.TokenCounter,
    observer pipeline.TokenObserver,
) *CompressionMiddleware

func (m *CompressionMiddleware) WrapPreNode(stepID string, step *pipeline.StepDefinition) graph.BeforeNodeCallback
func (m *CompressionMiddleware) WrapPostNode(stepID string, step *pipeline.StepDefinition) graph.AfterNodeCallback
// PostNode 返回 nil（压缩不需要 post 回调）
```

**来源代码**：`context_compressor.go:208-236` (`MakePreNodeCallback`)

---

### 6.4 创建 `PromptInjectionMiddleware`

```go
package flow

// PromptInjectionMiddleware 实现 pipeline.Middleware
// 在 PreNode 时重建 Layer1+2 系统消息
type PromptInjectionMiddleware struct {
    assembler pipeline.PromptAssembler  // ← 接口
}

func NewPromptInjectionMiddleware(assembler pipeline.PromptAssembler) *PromptInjectionMiddleware

func (m *PromptInjectionMiddleware) WrapPreNode(stepID string, step *pipeline.StepDefinition) graph.BeforeNodeCallback
func (m *PromptInjectionMiddleware) WrapPostNode(stepID string, step *pipeline.StepDefinition) graph.AfterNodeCallback
// PostNode 返回 nil
```

**来源代码**：`prompt_builder.go:80-103` (`MakePreNodeCallback`)

---

### 6.5 创建 `ArtifactRecordMiddleware`

```go
package flow

// ArtifactRecordMiddleware 实现 pipeline.Middleware
// 在 PostNode 时记录步骤产出物
type ArtifactRecordMiddleware struct {
    tracker pipeline.ArtifactTracker  // ← 接口
}

func NewArtifactRecordMiddleware(tracker pipeline.ArtifactTracker) *ArtifactRecordMiddleware

func (m *ArtifactRecordMiddleware) WrapPreNode(stepID string, step *pipeline.StepDefinition) graph.BeforeNodeCallback
// PreNode 返回 nil
func (m *ArtifactRecordMiddleware) WrapPostNode(stepID string, step *pipeline.StepDefinition) graph.AfterNodeCallback
```

**来源代码**：`artifacts.go:91-99` (`MakePostNodeCallback`)

---

### 6.6 创建 `helpers.go` — 辅助函数

从 `graph_helpers.go` 搬移，逻辑不变：

```go
package flow

func resolveToolSets(names []string, available map[string]tool.ToolSet, allowMissing bool) ([]tool.ToolSet, error)
func makeFallbackRouter(fallback map[string]string) graph.ConditionalFunc
func makeConfirmNode(stepID string, mode pipeline.AdvanceMode) graph.NodeFunc
func wrapToolsNode(base graph.NodeFunc) graph.NodeFunc
func confirmNodeID(stepID string) string   // stepID + ":confirm"
func toolsNodeID(stepID string) string     // stepID + ":tools"
func nextStepID(next string) string        // "" → graph.End
func clearPipelineErrorCode(...)            // PostNodeCallback
```

**来源**：`graph_helpers.go` 完整文件 (140 行)，所有函数原样搬移。

唯一改动：引用 `pipeline.StateKeyPipelineErrorCode` 和 `pipeline.AdvanceMode` 等类型从 `pipeline` 包导入。

---

### 6.7 创建 `graph.go` — GraphBuilder

```go
package flow

// GraphBuilder 实现 pipeline.FlowBuilder
// 构建带 next/fallback 条件路由的状态机（当前行为）
type GraphBuilder struct{}

func NewGraphBuilder() *GraphBuilder
func (b *GraphBuilder) Build(steps []*pipeline.StepDefinition, opts FlowOptions) (*graph.Graph, error)
```

**来源**：`graph_builder.go:99-318` (`BuildGraphFromPrompts`)

**重构拆解**为清晰的内部步骤：

```go
func (b *GraphBuilder) Build(steps, opts) (*graph.Graph, error) {
    sg := graph.NewStateGraph(...)
    
    // 步骤1: 创建所有节点
    for _, step := range steps {
        b.addLLMNode(sg, step, opts)       // 指令构建 + 选项组装
        b.addConfirmNode(sg, step, opts)   // 确认节点
        b.addToolsNode(sg, step, opts)     // 工具节点（如有）
    }
    
    // 步骤2: 连接所有边
    for _, step := range steps {
        b.addEdges(sg, step, opts)         // next/fallback/tools 边
    }
    
    // 步骤3: 设置入口和终点
    b.setEntryAndFinish(sg, steps)
    
    return sg.Compile()
}
```

**与当前的区别**：
1. **中间件注入通过 `opts.Middlewares`**，不再在函数内构造 65 行闭包
2. **指令构建**：如果有 PromptAssembler 中间件则用它，否则用 `buildInstruction` fallback
3. 每个子步骤是独立方法，可单独测试

---

### 6.8 创建 `chain.go` — ChainBuilder

```go
package flow

// ChainBuilder 实现 pipeline.FlowBuilder
// 线性执行：step1 → step2 → ... → END，忽略 fallback
type ChainBuilder struct{}

func NewChainBuilder() *ChainBuilder
func (b *ChainBuilder) Build(steps []*pipeline.StepDefinition, opts FlowOptions) (*graph.Graph, error)
```

**新增功能** — 最简流程形态：
- 按 `step` 字段排序线性连接
- 忽略所有 `fallback` 字段
- 忽略所有 `next` 字段（直接连接下一个）
- 保留 `advance` 模式（confirm/block/auto）
- 保留中间件注入

---

### 6.9 创建 `agent.go` — AgentBuilder

```go
package flow

// AgentBuilder 实现 pipeline.FlowBuilder
// 单 LLM 节点充当 router，每个阶段变成一个 tool
type AgentBuilder struct{}

func NewAgentBuilder() *AgentBuilder
func (b *AgentBuilder) Build(steps []*pipeline.StepDefinition, opts FlowOptions) (*graph.Graph, error)
```

**新增功能** — 动态 Agent 形态：
- 单个 LLM router 节点
- 每个 step 注册为一个 tool（`execute_stage_1.1`, `execute_stage_2.1` 等）
- LLM 自主决定调用哪个阶段
- 适合探索性、非线性任务

---

### 6.10 创建 `middleware_test.go`

测试用例：
- `TestMiddlewareChain_PreNode` — 按顺序执行，正确合并 State
- `TestMiddlewareChain_Empty` — 空中间件链返回 nil 回调
- `TestCompressionMiddleware_Triggered` — 超阈值时压缩
- `TestCompressionMiddleware_NotTriggered` — 未超阈值时跳过
- `TestPromptInjectionMiddleware_Rebuild` — 重建系统消息
- `TestArtifactRecordMiddleware_Record` — 步骤完成后记录

---

### 6.11 创建 `graph_test.go`

从现有 `graph_builder_test.go` 迁移：
- `TestGraphBuilder_WithFallbackConfirm` — 对应 `TestBuildGraphFromPrompts_WithFallbackConfirm`
- `TestGraphBuilder_ConfirmInterrupts` — 对应 `TestMakeConfirmNode_ConfirmInterrupts`
- 新增：`TestGraphBuilder_MultiStep` — 多步骤正确连接
- 新增：`TestGraphBuilder_EntryAndFinish` — 入口终点设置正确

---

### 6.12 创建 `chain_test.go`

测试用例：
- `TestChainBuilder_Linear` — 3 个步骤线性连接
- `TestChainBuilder_IgnoresFallback` — fallback 字段被忽略
- `TestChainBuilder_IgnoresNext` — next 字段被忽略，按顺序连接
- `TestChainBuilder_WithMiddleware` — 中间件正确注入

---

## 依赖关系

```
pkg/flow/
  types.go       → pipeline.Middleware (接口)
                 → model.Model, tool.ToolSet (框架类型)
  middleware.go  → pipeline.Compressor (接口)
                 → pipeline.TokenCounter (接口)
                 → pipeline.TokenObserver (接口)
                 → pipeline.PromptAssembler (接口)
                 → pipeline.ArtifactTracker (接口)
  helpers.go     → pipeline.AdvanceMode, pipeline.StateKeyPipelineErrorCode (常量)
                 → graph.*, tool.* (框架类型)
  graph.go       → pipeline.FlowBuilder (实现此接口)
                 → pipeline.StepDefinition (数据结构)
  chain.go       → pipeline.FlowBuilder (实现此接口)
  agent.go       → pipeline.FlowBuilder (实现此接口)
  
  依赖: pkg/pipeline (仅接口 + 常量)
  不依赖: pkg/token, pkg/memory, pkg/prompt, pkg/step (全部通过接口解耦)
```

## 验收标准

- [ ] `GraphBuilder` 编译通过，实现 `pipeline.FlowBuilder`，行为与当前 `BuildGraphFromPrompts` 一致
- [ ] `ChainBuilder` 编译通过，实现 `pipeline.FlowBuilder`
- [ ] `AgentBuilder` 编译通过，实现 `pipeline.FlowBuilder`
- [ ] 65 行 mega-closure 被拆解为 3 个独立的 Middleware
- [ ] 每个 Middleware 可独立测试
- [ ] `go test ./pkg/flow/...` 全部通过
- [ ] 现有 `graph_builder_test.go` 的测试用例全部迁移并通过
