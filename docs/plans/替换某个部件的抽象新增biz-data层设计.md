在 Kratos 和 DDD 的架构下，替换某个部件（如 `Memory`）的实现，核心依赖的是**依赖倒置原则（DIP）**和**依赖注入（Wire）**。

在 Agent Harness 场景中，Memory 的实现可能有很多种：基于 Redis 的短期记忆、基于 Milvus/Pinecone 的向量长期记忆、基于 Neo4j 的图记忆等。

要做到 **“替换实现而不修改任何业务代码”**，你需要遵循以下标准姿势：

---

### 第一步：在 `biz` 层定义“面向业务”的抽象（端口）

**核心原则**：接口定义在 `biz` 层，且**绝对不能包含任何技术细节**（不要出现 `Redis`、`Vector` 等字眼）。

```go
// internal/biz/memory/repository.go
package memory

import "context"

// Repository 是领域层定义的端口（Port）
// 它只关心“记忆”的业务行为，不关心底层是 Redis 还是 向量数据库
type Repository interface {
    // 存储一段记忆（情景/经验）
    Store(ctx context.Context, episode *Episode) error
    
    // 根据查询意图召回相关记忆
    Recall(ctx context.Context, query string, budget TokenBudget) ([]*Episode, error)
    
    // 记忆整合/遗忘（定期清理或合并低权重记忆）
    Consolidate(ctx context.Context) error
}
```

---

### 第二步：在 `data` 层提供不同的具体实现（适配器）

在 `data` 层，针对不同的技术栈，实现上述接口。

#### 实现 A：基于 Redis 的短期记忆
```go
// internal/data/memory_redis.go
package data

import "context"

type memoryRedisRepo struct {
    client *redis.Client
}

func NewMemoryRedisRepo(client *redis.Client) memory.Repository {
    return &memoryRedisRepo{client: client}
}

func (r *memoryRedisRepo) Store(ctx context.Context, episode *memory.Episode) error {
    // 将 Episode 序列化为 JSON，存入 Redis List 或 Hash
    return r.client.RPush(ctx, "agent:memory:"+episode.AgentID, episode.ToJSON()).Err()
}

func (r *memoryRedisRepo) Recall(ctx context.Context, query string, budget memory.TokenBudget) ([]*memory.Episode, error) {
    // 简单的 LRU 或最近 N 条召回逻辑
    // ...
}
```

#### 实现 B：基于 向量数据库（如 Milvus）的长期记忆
```go
// internal/data/memory_vector.go
package data

import "context"

type memoryVectorRepo struct {
    milvusClient *milvus.Client
    embedder     Embedder // 文本转向量的组件
}

func NewMemoryVectorRepo(mc *milvus.Client, emb Embedder) memory.Repository {
    return &memoryVectorRepo{milvusClient: mc, embedder: emb}
}

func (r *memoryVectorRepo) Store(ctx context.Context, episode *memory.Episode) error {
    // 1. 调用 embedder 将 episode.Content 转为 Vector
    // 2. 将 Vector 和元数据存入 Milvus
    return nil
}

func (r *memoryVectorRepo) Recall(ctx context.Context, query string, budget memory.TokenBudget) ([]*memory.Episode, error) {
    // 1. 将 query 转为 Vector
    // 2. 在 Milvus 中进行 ANN（近似最近邻）检索
    // 3. 返回结果
    return nil, nil
}
```

---

### 第三步：在 `biz` 层的 Usecase 中只依赖抽象

`MemoryUsecase` 根本不知道也不关心底层用的是 Redis 还是 Milvus。

```go
// internal/biz/memory/usecase.go
package memory

type MemoryUsecase struct {
    repo Repository // ⭐ 只依赖 biz 层定义的接口
}

func NewMemoryUsecase(repo Repository) *MemoryUsecase {
    return &MemoryUsecase{repo: repo}
}

func (uc *MemoryUsecase) Remember(ctx context.Context, content string) error {
    episode := &Episode{Content: content, Timestamp: time.Now()}
    return uc.repo.Store(ctx, episode) // 调用抽象接口
}
```

---

### 第四步：使用 Wire 进行“编译期”替换

当你想替换实现时，**只需要修改 Wire 的 ProviderSet，重新生成代码即可，业务代码一行都不用改。**

```go
// internal/data/provider.go
package data

import "github.com/google/wire"
import "kratos-agent/internal/biz/memory"

// 场景 1：我想用 Redis 实现
var MemoryProviderSet = wire.NewSet(
    NewMemoryRedisRepo,
    wire.Bind(new(memory.Repository), new(*memoryRedisRepo)), // 绑定接口与实现
)

// ---------------------------------------------------------
// 场景 2：老板说我们要上向量数据库，我只需要把上面的代码改成：
// var MemoryProviderSet = wire.NewSet(
//     NewMemoryVectorRepo,
//     wire.Bind(new(memory.Repository), new(*memoryVectorRepo)),
// )
```
执行 `wire` 命令后，`data` 层注入给 `MemoryUsecase` 的实例就自动从 Redis 变成了 Milvus。

---

### 进阶：Agent 场景下的“运行时”动态替换（组合模式）

在 Agent Harness 中，Memory 往往不是“非此即彼”的。一个成熟的 Agent 通常**同时需要**短期记忆（Redis）和长期记忆（Vector DB）。

这时候，你需要的是**运行时组合**，而不是单纯的替换。在 `data` 层实现一个**组合适配器（Composite Adapter）**：

```go
// internal/data/memory_composite.go
package data

// memoryCompositeRepo 组合了多个底层实现
type memoryCompositeRepo struct {
    shortTerm memory.Repository // 注入 Redis 实现
    longTerm  memory.Repository // 注入 Vector 实现
}

func NewMemoryCompositeRepo(short memory.Repository, long memory.Repository) memory.Repository {
    return &memoryCompositeRepo{shortTerm: short, longTerm: long}
}

func (r *memoryCompositeRepo) Store(ctx context.Context, episode *memory.Episode) error {
    // 核心逻辑：重要的记忆存长期，普通的存短期
    if episode.Importance > 0.8 {
        return r.longTerm.Store(ctx, episode)
    }
    return r.shortTerm.Store(ctx, episode)
}

func (r *memoryCompositeRepo) Recall(ctx context.Context, query string, budget memory.TokenBudget) ([]*memory.Episode, error) {
    // 核心逻辑：同时从短期和长期召回，然后合并去重
    shortEpisodes, _ := r.shortTerm.Recall(ctx, query, budget)
    longEpisodes, _ := r.longTerm.Recall(ctx, query, budget)
    
    return mergeAndDeduplicate(shortEpisodes, longEpisodes), nil
}
```

**Wire 注入组合实现：**
```go
var MemoryProviderSet = wire.NewSet(
    NewMemoryRedisRepo,
    NewMemoryVectorRepo,
    NewMemoryCompositeRepo, // 将 Redis 和 Vector 组合成一个新的 Repository
    wire.Bind(new(memory.Repository), new(*memoryCompositeRepo)),
)
```

---

### 总结

在 Kratos/DDD 中替换部件抽象的标准化套路：

1. **定规矩（biz 层）**：定义纯粹的、面向业务的 `interface`。
2. **做实现（data 层）**：针对不同的技术栈（Redis/Milvus/Kafka）编写 `struct` 实现该接口。
3. **绑关系（Wire）**：通过 `wire.Bind` 将接口与具体的 `struct` 绑定。
4. **换实现**：
   * **编译期替换**：修改 Wire 的 `ProviderSet`，换一个 `NewXxxRepo`。
   * **运行时组合**：在 `data` 层写一个 `Composite` 结构体，内部聚合多个实现，对外依然暴露同一个 `interface`。

这种设计完美契合了**六边形架构（端口与适配器）** 的精髓：**核心业务逻辑（端口）被牢牢保护在中心，外围的技术细节（适配器）可以像插拔 U 盘一样随意替换。**