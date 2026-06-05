# Decision
Pai provider 首版采用“任务型视频适配 + 自动图片上传”的实现方式。

# Why
- 官方文档要求图生视频和首尾帧先准备 `img_id`
- 现有 `omnigo` 架构已经适合表达“提交任务 + 查询状态”的模式
- 自动上传可以降低接入成本，避免每个调用方都重复写上传流程

# Tradeoffs
- adaptor 逻辑更重，需要处理图片读取、上传和模式判断
- 如果业务想复用已有 `img_id` 做更细的缓存控制，需要通过 `extra` 传入覆盖

# Alternatives Considered
- 只支持调用方手动传 `img_id`
- 先只做文生视频，后续再加图片能力

# Final Outcome
保留统一入口，支持显式 `img_id`，并在缺少 `img_id` 时自动上传图片。
