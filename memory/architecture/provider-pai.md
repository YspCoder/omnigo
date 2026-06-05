# Pai Provider Architecture

## Scope
- Provider name: `pai`
- Base URL: `https://app-api.pixverseai.cn`
- Current modes:
  - `text-to-video` -> `/openapi/v2/video/text/generate`
  - `image-to-video` -> `/openapi/v2/video/img/generate`
  - `transition` -> `/openapi/v2/video/transition/generate`
  - `extend` -> `/openapi/v2/video/extend/generate`
  - `swap` -> `/openapi/v2/video/swap/generate`
  - `multi-transition` -> `/openapi/v2/video/multi_transition/generate`
  - `mimic` -> `/openapi/v2/video/mimic/generate`
  - `lip-sync` -> `/openapi/v2/video/lip_sync/generate`
  - `mask-selection` -> `/openapi/v2/video/mask/selection`
  - `sound-effect` -> `/openapi/v2/video/sound_effect/generate`
  - `restyle` -> `/openapi/v2/video/restyle/generate`
  - `restyle/list` -> `ListTasks(mode=restyle)` -> `/openapi/v2/video/restyle/list`
  - `modify` -> `/openapi/v2/video/modify/generate`

## Flow
```text
MediaRequest
  -> PaiAdaptor.resolveMode
  -> PaiAdaptor.buildPayload
     -> if img mode and no img_id: upload image -> img_id
     -> if transition mode and no frame ids: upload two images -> first_frame_img / last_frame_img
     -> if multi-transition and no img_id in segments: upload each segment image
     -> if mimic and no img_id: upload reference image
     -> if swap.auto_mask_selection: call mask-selection first
  -> POST generate endpoint
  -> return unified MediaResponse{TaskID}
  -> TaskStatus polls /openapi/v2/video/result/{video_id}
  -> map provider status to unified TaskStatusOutput

mask-selection
  -> synchronous helper call
  -> returns keyframe + mask candidates immediately
  -> current adapter maps `keyframe_url` to `MediaResponse.URL`
  -> preserves full provider JSON in `MediaResponse.Text`

restyle/list
  -> ListTasks query path
  -> provider list items are mapped into TaskListItem as available presets

modify
  -> task-style generation call
  -> expects prompt references plus img_ids / mask_ids / keyframe_ids
  -> adapter can upload image inputs and expand them into img_ids
```

## Dependencies
- `adapter/pai.go`: provider protocol mapping
- `adapter/registry.go`: provider registration
- `utils.MediaPromptWithSystem`: prompt 合并
- `utils.ParseExtraImageInputs`: 图片输入提取

## Design Reason
- Pai 官方是典型异步视频任务接口，适合直接映射到现有 `Media -> TaskStatus -> StreamMedia` 抽象。
- 图片类输入需要先转成 `img_id`，因此把上传逻辑封装在 adaptor 内部，减少调用方额外步骤。
- 新增独立能力依旧保持同一 adaptor，而不是拆多个 provider，避免调用侧感知官方子产品差异。
- 对于不符合任务轮询模型的辅助接口，优先保留原始结果，而不是强造统一任务语义。
- 对于“查询列表但不是任务列表”的接口，允许借用 `ListTasks` 容器承载可用项，只要在文档里明确语义。

## Tradeoffs
- 优点：调用方只需要传统一 `MediaRequest`，不必手动走“上传 -> 生成 -> 轮询”三段流程。
- 缺点：adaptor 内部承担了更多 I/O 和模式推断逻辑，后续若能力继续扩展，建议抽取公共任务型视频层。
