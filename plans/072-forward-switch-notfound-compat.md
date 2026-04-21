# 072 规则开关 notfound 兼容修复

## Checklist
- [x] 复现并定位规则开关报错 `service ... notfound` 的兼容缺口。
- [x] 修复后端 missing-service 识别逻辑，兼容无空格的 `notfound` 返回。
- [x] 为该兼容路径补充回归测试。
- [ ] 运行后端构建与测试验证修复。
