# pkg/step — 步骤加载与阶段过滤

> 从 `graph_builder.go:LoadPrompts` 和 `frontmatter.go` 抽出，提供多种步骤加载策略（文件系统/内存/过滤/组合），支持阶段隔离和加载时校验。

---

## 来源文件映射

| 当前位置 | 迁移到本包 |
|---------|-----------|
| `pkg/pipeline/graph_builder.go` → `LoadPrompts()` (行 56-96) | `loader.go` |
| `pkg/pipeline/frontmatter.go` → `LoadPrompt()` / `ParsePrompt()` (行 78-125) | `parser.go` |
| 无（新增功能） | `validator.go` |

---

## Phase 5 原子任务

### 5.1 创建 `loader.go` — FileStepLoader

```go
package step

// FileStepLoader 实现 pipeline.StepLoader 接口
// 从文件系统目录加载步骤定义
type FileStepLoader struct {
    fs  pipeline.FileSystem  // ← 接口，不直接 filepath.WalkDir
    dir string
}

func NewFileStepLoader(fs pipeline.FileSystem, dir string) *FileStepLoader
func (l *FileStepLoader) Load() ([]*pipeline.StepDefinition, error)
```

**来源代码**：`graph_builder.go:56-96` (`LoadPrompts`)

**改动**：
- `filepath.WalkDir` → 使用 `fs.ReadDir` 递归遍历
- 跳过 `templates/` 和 `system/` 子目录（保持不变）
- 跳过 `_` 前缀文件（保持不变）
- 按 `step` 字段排序（保持不变）

---

### 5.2 创建 `FilteredStepLoader` — 按前缀过滤

```go
package step

// FilteredStepLoader 包装另一个 StepLoader，按 step ID 前缀过滤
type FilteredStepLoader struct {
    inner  pipeline.StepLoader
    prefix string  // e.g. "1." 只加载设计阶段
}

func NewFilteredStepLoader(inner pipeline.StepLoader, prefix string) *FilteredStepLoader
func (l *FilteredStepLoader) Load() ([]*pipeline.StepDefinition, error)
// 逻辑：调用 inner.Load()，过滤出 step ID 以 prefix 开头的
```

**新增功能** — 解决"无法按阶段加载"的问题。

使用示例：
```go
// 只加载阶段一（1.1-1.6）
loader := step.NewFilteredStepLoader(
    step.NewFileStepLoader(osFS, "prompts"),
    "1.",
)
```

---

### 5.3 创建 `CompositeStepLoader` — 合并多来源

```go
package step

// CompositeStepLoader 合并多个 StepLoader 的结果
type CompositeStepLoader struct {
    loaders []pipeline.StepLoader
}

func NewCompositeStepLoader(loaders ...pipeline.StepLoader) *CompositeStepLoader
func (l *CompositeStepLoader) Load() ([]*pipeline.StepDefinition, error)
// 逻辑：依次调用每个 loader，合并结果，按 step 排序，检查重复
```

使用示例：
```go
// 从两个目录加载
loader := step.NewCompositeStepLoader(
    step.NewFileStepLoader(osFS, "prompts/design"),
    step.NewFileStepLoader(osFS, "prompts/implementation"),
)
```

---

### 5.4 创建 `InMemoryStepLoader` — 测试用

```go
package step

// InMemoryStepLoader 从内存中提供步骤定义，用于测试
type InMemoryStepLoader struct {
    steps []*pipeline.StepDefinition
}

func NewInMemoryStepLoader(steps ...*pipeline.StepDefinition) *InMemoryStepLoader
func (l *InMemoryStepLoader) Load() ([]*pipeline.StepDefinition, error)
```

---

### 5.5 创建 `parser.go` — 解析逻辑

```go
package step

func LoadStep(fs pipeline.FileSystem, path string) (*pipeline.StepDefinition, error)
func ParseStep(content string) (pipeline.Frontmatter, string, error)
```

**来源**：`frontmatter.go:78-95` (`LoadPrompt`) + `frontmatter.go:98-125` (`ParsePrompt`)

**改动**：
- `os.ReadFile(path)` → `fs.Open(path)` 使用 FileSystem 接口
- 函数名从 `LoadPrompt`/`ParsePrompt` → `LoadStep`/`ParseStep`
- 逻辑完全不变

---

### 5.6 创建 `validator.go` — 引用校验

```go
package step

// ValidateReferences 检查所有 next/fallback 引用的 stepID 是否存在
func ValidateReferences(steps []*pipeline.StepDefinition) []ValidationError

type ValidationError struct {
    StepID    string // 当前步骤
    Field     string // "next" 或 "fallback.{code}"
    Reference string // 引用的目标 stepID
    Message   string // 错误描述
}
```

**新增功能** — 解决"删掉步骤 1.3 后 1.2 的 fallback 静默指向不存在节点"的问题。

校验规则：
1. `next` 字段引用的 stepID 必须存在（除空/null 外）
2. `fallback` map 中每个 value 引用的 stepID 必须存在
3. 不允许 `next` 指向自身（死循环）
4. 警告：存在孤立步骤（没有任何 next/fallback 指向它，且不是入口步骤）

---

### 5.7 创建 `loader_test.go`

测试用例：
- `TestFileStepLoader_Normal` — 正常加载，按 step 排序
- `TestFileStepLoader_SkipSystemDir` — 跳过 system/ 目录
- `TestFileStepLoader_SkipUnderscorePrefix` — 跳过 _ 前缀文件
- `TestFilteredStepLoader_ByPrefix` — 过滤后只返回匹配的步骤
- `TestCompositeStepLoader_Merge` — 合并两个 loader 的结果
- `TestCompositeStepLoader_DuplicateStep` — 重复 stepID 报错
- `TestInMemoryStepLoader` — 内存 loader 返回预设步骤

Mock 依赖：`testing/fstest.MapFS` 作为 `pipeline.FileSystem`

---

### 5.8 创建 `validator_test.go`

测试用例：
- `TestValidateReferences_Valid` — 所有引用正确，返回空
- `TestValidateReferences_DanglingNext` — next 指向不存在的步骤
- `TestValidateReferences_DanglingFallback` — fallback 指向不存在的步骤
- `TestValidateReferences_SelfLoop` — next 指向自身
- `TestValidateReferences_NullNext` — next 为空（合法，表示终点）

---

## 依赖关系

```
pkg/step/
  loader.go    → pipeline.StepLoader (实现此接口)
               → pipeline.FileSystem (接口)
               → pipeline.StepDefinition (数据结构)
  parser.go    → pipeline.Frontmatter (数据结构)
               → pipeline.FileSystem (接口)
  validator.go → pipeline.StepDefinition (数据结构)
  
  依赖: pkg/pipeline (仅接口 + 数据结构)
  不依赖: pkg/token, pkg/memory, pkg/prompt, pkg/flow
```

## 验收标准

- [ ] `FileStepLoader` 编译通过，实现 `pipeline.StepLoader`
- [ ] `FilteredStepLoader` 可按前缀过滤阶段
- [ ] `ValidateReferences` 能检测悬空引用
- [ ] 不包含任何 `os.ReadFile` / `filepath.WalkDir` 直接调用
- [ ] `go test ./pkg/step/...` 全部通过
