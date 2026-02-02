# 开发日志

> 本项目 Fork 自 [kaqijiang/new-api](https://github.com/kaqijiang/new-api)，在此基础上进行定制开发。

---

## 2026-02-02: 添加 Semantic Scholar 渠道

### 需求背景
需要在 new-api 项目中添加 Semantic Scholar API 代理功能，实现：
- Semantic Scholar API 透传代理
- 多 API Key 自动轮询
- 后台管理渠道和 API Key
- 调用记录和日志
- 代理支持（HTTP/HTTPS/SOCKS5）

### 实现方案对比

| 方案 | 描述 | 优点 | 缺点 |
|-----|------|------|------|
| **方案 A** | 独立中间件 | 完全独立，不侵入原项目 | 无法复用渠道管理、日志、多 Key 轮询等现有功能 |
| **方案 B** | 集成渠道系统 | 复用现有基础设施，功能完整 | 需要修改原项目代码 |

**采用方案 B（集成渠道系统）**，原因：
1. 复用现有的渠道管理 UI、多 Key 轮询、代理支持等基础设施
2. 调用日志自动记录到现有日志系统
3. 修改集中在新增文件，对原项目侵入性小
4. 便于后续维护和与上游同步

### 修改文件清单

#### 后端 (Go)

| 文件 | 修改类型 | 说明 |
|------|---------|------|
| `constant/channel.go` | 修改 | 添加 `ChannelTypeSemanticScholar = 58` 及 Base URL、名称映射 |
| `constant/api_type.go` | 修改 | 添加 `APITypeSemanticScholar` 常量 |
| `common/api_type.go` | 修改 | 添加 ChannelType → APIType 映射 |
| `relay/channel/semanticscholar/adaptor.go` | **新建** | 透传适配器，使用 x-api-key 认证 |
| `relay/channel/semanticscholar/constant.go` | **新建** | 渠道常量 |
| `relay/relay_adaptor.go` | 修改 | 注册适配器 |
| `controller/semantic_scholar.go` | **新建** | 透传控制器，支持 Key 轮询、日志、代理 |
| `router/s2-router.go` | **新建** | `/s2/*` 路由配置 |
| `router/main.go` | 修改 | 注册 S2 路由 |
| `model/channel.go` | 修改 | 添加 `GetEnabledChannelByType` 函数 |

#### 前端 (React)

| 文件 | 修改类型 | 说明 |
|------|---------|------|
| `web/src/constants/channel.constants.js` | 修改 | 添加渠道选项 (value: 58) |

### 使用方法

#### 1. 后台添加渠道
- **类型**：选择 `Semantic Scholar`
- **密钥**：填写 API Key（多个用换行分隔，自动轮询）
- **Base URL**：留空（默认 `https://api.semanticscholar.org`）
- **代理**：可选，在渠道设置中配置

#### 2. 用户调用示例
```bash
# 搜索论文
curl "http://your-server/s2/graph/v1/paper/search?query=machine+learning" \
  -H "Authorization: Bearer <user_token>"

# 获取论文详情
curl "http://your-server/s2/graph/v1/paper/{paper_id}?fields=title,abstract" \
  -H "Authorization: Bearer <user_token>"
```

#### 3. 支持的 API 路径映射

| 用户请求 | 上游 URL |
|---------|----------|
| `/s2/graph/v1/*` | `https://api.semanticscholar.org/graph/v1/*` |
| `/s2/recommendations/v1/*` | `https://api.semanticscholar.org/recommendations/v1/*` |
| `/s2/datasets/v1/*` | `https://api.semanticscholar.org/datasets/v1/*` |

### 功能特性
- ✅ 透传所有请求参数和响应
- ✅ 多 Key 自动轮询
- ✅ 调用日志记录
- ✅ 代理支持 (HTTP/HTTPS/SOCKS5)
- ✅ Go 编译验证通过

---

## 分支管理策略

建议创建独立的功能分支，便于后续与上游同步：

```bash
# 创建并切换到功能分支
git checkout -b feature/semantic-scholar

# 提交修改
git add .
git commit -m "feat: add Semantic Scholar channel support"

# 后续同步上游更新
git remote add upstream https://github.com/kaqijiang/new-api.git
git fetch upstream
git checkout main
git merge upstream/main
git checkout feature/semantic-scholar
git rebase main
```
