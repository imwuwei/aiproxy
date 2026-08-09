## ADDED Requirements

### Requirement: OpenAI 格式代理入口

系统 SHALL 提供统一 OpenAI 格式的 HTTP 代理服务，默认监听 127.0.0.1，端口可配置。至少支持以下路由：`POST /v1/chat/completions`、`GET /v1/models`、`POST /v1/embeddings`。代理收到的请求被转发到厂商渠道，响应以 OpenAI 兼容格式返回。

#### Scenario: 调用聊天补全接口

- **WHEN** 客户端向 `POST /v1/chat/completions` 发送 OpenAI 格式的聊天请求
- **THEN** 系统将请求转发至可用渠道并将响应以 OpenAI 格式返回

#### Scenario: 查询模型列表

- **WHEN** 客户端向 `GET /v1/models` 发起请求
- **THEN** 系统返回聚合后的统一模型列表（OpenAI 格式）

#### Scenario: 调用嵌入接口

- **WHEN** 客户端向 `POST /v1/embeddings` 发送 OpenAI 格式的嵌入请求
- **THEN** 系统将请求转发至可用渠道并将响应以 OpenAI 格式返回

### Requirement: 流式响应透传

系统 SHALL 支持 Server-Sent Events（SSE）流式响应。当请求包含 `stream: true` 时，系统 SHALL 以流式方式转发厂商响应，逐 chunk 透传。

#### Scenario: 流式聊天请求

- **WHEN** 客户端发起 `stream: true` 的聊天补全请求
- **THEN** 系统以 SSE 格式逐块返回厂商响应，直到流结束

#### Scenario: 流式请求渠道切换

- **WHEN** 流式请求的优先渠道在响应首包前失败
- **THEN** 系统切换到下一可用渠道重新发起请求，客户端无感知

### Requirement: 请求鉴权

系统 SHALL 支持代理访问令牌（Bearer Token）鉴权。开启时，请求 MUST 携带 `Authorization: Bearer <token>` 头，否则返回 401。令牌可在设置中修改；鉴权可关闭以便本机使用。

#### Scenario: 携带有效令牌

- **WHEN** 客户端请求携带有效令牌
- **THEN** 请求被正常处理并转发

#### Scenario: 缺失或无效令牌

- **WHEN** 客户端请求缺失或携带无效令牌
- **THEN** 系统返回 401 拒绝访问

#### Scenario: 鉴权关闭

- **WHEN** 用户在设置中关闭鉴权
- **THEN** 所有请求无需令牌即可访问代理

### Requirement: 错误响应格式

当所有可用渠道均失败或请求不合法时，系统 SHALL 返回 OpenAI 兼容格式的错误响应，包含错误类型与消息。

#### Scenario: 模型不存在

- **WHEN** 客户端请求的模型在所有启用渠道中均不可用
- **THEN** 系统返回 404 及 OpenAI 格式错误消息

#### Scenario: 所有渠道失败

- **WHEN** 目标模型的所有渠道均请求失败
- **THEN** 系统返回聚合错误，消息中包含各渠道失败原因

### Requirement: 厂商格式转换适配

系统 SHALL 内置 Anthropic 与 Gemini 适配器。对于非 OpenAI 格式的厂商（Anthropic messages API、Gemini API），系统 SHALL 在转发时将 OpenAI 请求体转换为厂商格式，并将厂商响应转换回 OpenAI 格式。

#### Scenario: 转发到 Anthropic 渠道

- **WHEN** 客户端发送 OpenAI 格式聊天请求且路由目标为 Anthropic 渠道
- **THEN** 系统转换为 Anthropic messages 格式转发，并将响应转换回 OpenAI 格式返回

#### Scenario: 转发到 Gemini 渠道

- **WHEN** 客户端发送 OpenAI 格式聊天请求且路由目标为 Gemini 渠道
- **THEN** 系统转换为 Gemini generateContent 格式转发，并将响应转换回 OpenAI 格式返回