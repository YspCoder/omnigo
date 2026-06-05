# 当前项目目标
- 维护一个统一的 Go 多模型 / 多媒体 provider 访问层
- 用统一 DTO 和 adaptor/relay 架构屏蔽不同厂商差异

# 已完成模块
- OpenAI / Ali / Google / Ark / Vidu / Kling 适配
- 新增 Pai / PixVerse 视频任务适配
- Pai 独立能力已扩展到 extend、swap、multi-transition、mimic、lip-sync
- Pai 已新增 mask-selection、sound-effect、restyle
- Pai 已新增 restyle/list 查询封装与 modify 生成接入
- 统一 `Media` / `TaskStatus` / `StreamMedia` 调用链

# 当前架构
- `adapter/` 负责厂商协议适配
- `relay/` 负责统一调度
- `dto/` 提供统一请求响应结构
- `examples/` 提供最小运行示例
- `memory/` 维护长期项目上下文

# 未完成任务
- Pai 其余能力接口尚未接入（真实环境验证、可能的后续漂移修正等）
- README 中的 provider 总览与 registry 历史项还有进一步对齐空间

# 技术债务
- 不同视频 provider 的图片输入解析逻辑已有一些重复
- 任务型 provider 的状态码映射还可以进一步抽象

# 当前 blocker
- 无阻塞；`go test ./...` 已通过

# 下一步建议
- 抽取通用“任务型视频 provider”基类
- 给 Pai 增加更多模式和真实环境验证
