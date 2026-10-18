# TarsMem0 Changelog

## v1.0.0
- Initial release: MCP integration for Mem0 with `mem0_search` and `mem0_add` tools
- Supports stdio and HTTP modes
- Basic health check and tool listing endpoints

## v1.1.0
- 可配置请求超时（TARS_MCP_TIMEOUT 环境变量）
- 添加 User-Agent 请求头标识
- 添加指数退避重试机制（TARS_MCP_MAX_RETRY，默认3次）
- 启动时配置验证（endpoint 格式检查）
- 改进 graceful shutdown（10秒超时）

## v1.2.0
- 新增 mem0_get 工具：按 ID 获取单条记忆
- HTTP 与 stdio 双模式均支持
- 添加对应单元测试

## v1.3.0
- 新增 mem0_delete 工具：按 ID 删除记忆
- HTTP 与 stdio 双模式均支持
- 添加对应单元测试

## v1.4.0
- 新增 /version 端点：返回版本与 Go 运行时信息
- 新增 /metrics 端点：返回内存与 goroutine 指标
- 添加对应单元测试

## v1.5.0
- 请求日志中间件：记录方法、路径、耗时、User-Agent
- 安全响应头：X-Content-Type-Options, X-Frame-Options
- CORS 预检支持

## v1.6.0
- 添加 Makefile：build / test / clean / run 目标
- 构建时注入版本号 ldflags

## v1.7.0
- 添加多阶段 Dockerfile
- Alpine 基础镜像，支持容器化部署

## v1.8.0
- 添加 GitHub Actions CI 工作流
- 自动跑测试与构建

## v1.9.0
- 错误信息小白话：所有报错文案去技术化
- "配置验证失败" → "配置检查没通过"
- "缺少参数" → "需要参数"
- "调用失败" → "请求没成功"

## v1.10.0
- README 增加 Build 章节（make + docker）
- 整合历史优点：安全头、重试、日志、配置验证、小白话
- 测试覆盖 16 个用例，-race 全绿
