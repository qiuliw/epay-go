# 异步 Worker 固定并发改造实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**目标：** 将商户通知、主动查单从「单协程 + 固定间隔串行轮询」改为「扫库投递 + 固定并发池 + 退避调度」，消除 IO 串行阻塞浪费，发挥 Go 高并发优势，降低尾延迟与互相拖慢。

**架构：** 保留数据库待办表作为可靠真相来源（`next_notify_at` / `next_query_at`）；调度器负责捞取到期任务；固定数量 worker 并发执行 HTTP IO；既有退避间隔继续控制重试节奏。可选：内存 channel 即时唤醒，轮询仅作兜底。

**技术栈：** Go 1.21+, Gin, GORM, PostgreSQL（现有退避字段复用，不强制引入 Redis/MQ）

---

## 问题背景与动机

### 现状

进程启动时各起 **1 个常驻协程**：

| Worker | 文件 | 轮询间隔 | 批大小 | 执行方式 |
|--------|------|----------|--------|----------|
| 商户通知 | `internal/service/notify.go` | 10s | 50 | 串行 `SendNotify` |
| 主动查单 | `internal/service/order_query.go` | 5s | 50 | 串行 `queryAndProcess` |

退避策略本身是合理的：

- 通知：`NotifyRetryIntervals`（立即 / 1m / 3m / 20m / 1h / 2h）
- 查单：`QueryIntervals`（20s / 30s / 60s / 120s / 300s）

问题不在退避，而在 **执行模型**。

### 核心判断（改造依据）

1. **轮询 IO 导致串行阻塞浪费**  
   通知下游、查询上游都是 HTTP IO。当前一个协程里一个一个处理，大量时间堵在网络等待上，CPU 与调度资源闲置。

2. **没有发挥多线程 / 多协程作用**  
   单轮询协程串行跑完整批，等于主动放弃并行度；一单慢（HTTP 超时可达 10s）会拖慢同批后续所有单（队头阻塞）。

3. **Go 的高并发效果更好**  
   Go 在 OS 线程之上用 goroutine 做 M:N 调度，特别适合 IO 密集场景：固定并发度下可同时挂起大量等待，用更少线程覆盖更多在途请求，进一步减少阻塞浪费。应采用「固定并发 + 退避间隔」，而不是「固定时间串行轮询处理」。

4. **固定时间轮询还有额外滞后**  
   即使任务已到期，也要等到下一个 tick；处理超时不会并行再开一轮，只能拖长本轮、丢弃中间 tick。

### 改造原则

- **保留**：DB 待办 + 退避间隔（可靠、可恢复、重启不丢）
- **改变**：串行执行 → 固定并发池；傻等 tick → 处理完可立即再捞 / channel 唤醒
- **不做（本阶段）**：不上 Kafka/RabbitMQ；不把业务状态只放在内存队列

---

## 目标架构

```
┌─────────────────┐     到期待办      ┌──────────────┐
│  PostgreSQL     │ ───────────────► │  Dispatcher  │  定时兜底扫库
│  orders 待办字段 │ ◄── 写回退避/状态 ─ │  (1 goroutine)│  + 可选即时唤醒
└─────────────────┘                   └──────┬───────┘
                                             │ jobs
                                             ▼
                                      ┌──────────────┐
                                      │ Worker Pool  │  固定 N 并发
                                      │ (N goroutine)│  执行 HTTP IO
                                      └──────────────┘
```

### 关键设计点

1. **固定并发 N**（建议通知 16、查单 16，可配置）  
   有上限，避免每单一个协程打爆下游或耗尽连接。

2. **退避间隔不变**  
   失败仍写 `next_*_at`，由调度决定何时再次入队。

3. **Dispatcher 职责**  
   - 周期性扫库（间隔可缩短，如 1–2s，或处理完空闲再扫）  
   - 将到期任务投入带缓冲的 job channel  
   - 需防重复投递（见 Task 2 租约/认领）

4. **即时唤醒（P1）**  
   支付成功路径已有 `go SendNotify`；可改为向 notify 池投递，失败仍回 DB 退避。查单一般靠退避，可不强依赖唤醒。

5. **优雅关闭**  
   `ctx.Done()` 后停止投递，worker 消化完在途或有超时退出。

---

## 功能优先级

### 🔴 P0 - 必须

1. 抽取通用 worker pool（或通知/查单各一套固定并发循环）
2. 通知 Worker 改为固定并发执行
3. 查单 Worker 改为固定并发执行
4. 扫库认领防重（避免多实例或同批重复通知/查单）
5. 配置项：并发数、扫库间隔

### 🟡 P1 - 重要

6. 支付成功 / 查单成功后 channel 唤醒，降低首通知延迟
7. 指标日志：在途数、批次耗时、队头等待

### 🟢 P2 - 后续

8. Redis 分布式锁 / 队列（多实例水平扩展时）
9. 按商户或下游域名限流

---

## Task 1: 引入可配置的 Worker 参数

**文件：**
- Modify: `internal/config/config.go`
- Modify: `config.example.yaml`
- Modify: `.env.example`（如有对应项）

### 步骤 1: 增加配置结构

```go
type WorkerConfig struct {
	NotifyConcurrency int `mapstructure:"notify_concurrency"`
	NotifyPollInterval int `mapstructure:"notify_poll_interval_sec"` // 秒
	QueryConcurrency  int `mapstructure:"query_concurrency"`
	QueryPollInterval int `mapstructure:"query_poll_interval_sec"`
}
```

默认建议：

| 配置 | 默认 |
|------|------|
| `notify_concurrency` | 16 |
| `notify_poll_interval_sec` | 2 |
| `query_concurrency` | 16 |
| `query_poll_interval_sec` | 2 |

非法值（≤0）回退到默认。

### 步骤 2: 启动时注入

`cmd/server/main.go` 创建 Notify/Query service 时传入配置。

---

## Task 2: 待办认领（防重复投递）

串行时「捞出来再处理」重复风险较小；固定并发 + 更勤扫库后，必须显式认领。

**文件：**
- Modify: `internal/repository/order.go`
- Modify: `internal/model/order.go`（若需新字段则加；优先用现有字段表达）

### 推荐方案（单实例优先，侵入小）

**乐观认领 / 短租约：**

1. `GetPendingNotifyOrders` 改为在事务中：
   - `SELECT ... FOR UPDATE SKIP LOCKED LIMIT N`（PostgreSQL）
   - 将 `next_notify_at` 临时推到「现在 + 租约秒数」（如 60s），或设置处理中状态
   - 返回这批订单
2. 处理成功：写最终通知状态
3. 处理失败：按退避写真正的 `next_notify_at`
4. 进程崩溃：租约到期后可再次被捞起

查单同理操作 `next_query_at`。

### 步骤验收

- 并发 8 下同一 `trade_no` 不会被两个 worker 同时 `SendNotify`
- 杀进程后，未完成单在租约后可被重新捞取

---

## Task 3: 通用固定并发执行器（可选抽取）

**文件：**
- Create: `internal/worker/pool.go`（若希望两处复用）
- 或直接在 notify/query 内各实现，保持文件局部性

### 参考骨架

```go
type Pool struct {
	concurrency int
	jobs        chan func(context.Context)
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.concurrency; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-p.jobs:
					job(ctx)
				}
			}
		}()
	}
}

func (p *Pool) Submit(job func(context.Context)) {
	p.jobs <- job
}
```

注意：

- `jobs` 带缓冲（如 `concurrency*2`），缓冲满时 Dispatcher 应阻塞或跳过本轮，避免无界内存
- 不要在 Submit 里再套一层无限 `go`

---

## Task 4: 改造商户通知 Worker

**文件：**
- Modify: `internal/service/notify.go`
- Modify: `cmd/server/main.go`

### 当前问题代码

```go
case <-ticker.C:
    s.processNotifyQueue() // 内部 for 串行 SendNotify
```

### 目标行为

1. Dispatcher 协程：按 `NotifyPollInterval` 扫库认领一批  
2. 将每单封装为 job 提交到池（固定 `NotifyConcurrency`）  
3. Worker 内调用现有 `SendNotify`（退避逻辑保留）  
4. `ctx` 取消时停止扫库，等待在途结束（可设 shutdown timeout）

### 步骤

1. 拆分 `processNotifyQueue`：`claimPendingNotify` + `executeNotify(order)`  
2. 用 worker 池并发 `executeNotify`  
3. 保留 `NotifyRetryIntervals` 写库逻辑不动  
4. 日志增加 `concurrency` / 本批 claim 数量 / 单笔耗时（可选）

### 验收

- 人为将下游 notify 接口 sleep 3s，50 笔待通知时总耗时应接近 `ceil(50/N)*3s`，而非 `50*3s`
- 失败单仍按退避出现在后续扫库中

---

## Task 5: 改造主动查单 Worker

**文件：**
- Modify: `internal/service/order_query.go`

与 Task 4 同构：

1. 认领 `GetPendingQueryOrders`  
2. 固定并发执行 `queryAndProcess`  
3. 保留 `QueryIntervals` / `scheduleNext`  
4. 查单成功后的 `go SendNotify`：P0 可暂留；P1 改为投入通知池，避免再开无界 goroutine

### 验收

- 上游 query 变慢时，多笔查单可并行推进
- 已支付订单不会因并发查单被重复入账（依赖现有 `ProcessPayNotify` 幂等；需回归验证）

---

## Task 6: 即时唤醒（P1）

**文件：**
- Modify: `internal/service/notify.go`
- Modify: `internal/service/order.go`（支付成功路径）
- Modify: `internal/service/order_query.go`

### 步骤

1. `NotifyService` 增加非阻塞 `Wake(tradeNo string)` 或直接 `SubmitOrder(*Order)`  
2. 支付成功、主动查单确认支付后调用唤醒  
3. 通道满则丢弃唤醒，依赖扫库兜底（可接受）

目的：去掉「成功后还要等下一个 poll tick」的额外滞后；扫库仍负责重试与恢复。

---

## Task 7: 测试与回归

**文件：**
- Create: `internal/service/notify_worker_test.go`（可用 httptest + fake clock 或短间隔）
- Create: `internal/service/order_query_worker_test.go`
- 手工：多笔测试支付 + 慢 notify mock

### 必测项

1. 并发通知耗时对比（串行 vs 固定 N）  
2. 同一订单不会双通知成功（认领）  
3. 崩溃恢复：处理中被杀，租约后重试  
4. 退避间隔仍生效  
5. `ProcessPayNotify` 幂等（并发查单）

---

## 实现顺序建议

```
Task 1 配置
  → Task 2 认领（并发安全基础）
  → Task 3 Pool（可选）
  → Task 4 通知改造
  → Task 5 查单改造
  → Task 7 测试
  → Task 6 唤醒（P1）
```

---

## 非目标（本计划不做）

- 用 Redis List/Stream 替代 DB 待办作为唯一队列
- 多机房 / 多实例分布式调度（可在认领 SQL 用 `SKIP LOCKED` 预留，但不做完整方案）
- 改变易支付下游协议或上游适配器逻辑

---

## 成功标准

1. 通知与查单均以 **固定并发** 执行 IO，不再单协程串行啃整批  
2. **退避间隔** 行为与现网一致（次数、时间档位不变）  
3. 慢下游时，批处理耗时近似按并发度线性改善  
4. 单实例下无重复通知 / 无重复入账  
5. 代码与注释风格与现有 `internal/service` 保持一致
