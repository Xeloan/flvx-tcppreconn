# 072 Dash 转发开关 not found 修复

## Checklist

- [x] 定位规则卡片开关报 `service1_1_0 not found` 的原因，确认问题出在 Dash 模式下暂停/恢复仍依赖本地 registry。
- [x] 为 Dash 模式补齐暂停/恢复服务的控制逻辑，直接操作 Dash API 并同步本地 paused 元数据。
- [x] 增加针对 Dash 模式暂停/恢复路径的回归测试。
- [x] 运行相关后端/agent 测试并确认通过。
