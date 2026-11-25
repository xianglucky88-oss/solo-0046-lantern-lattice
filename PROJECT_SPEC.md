# 灯格：后端契约

## 领域流程

`Grid.Size` 每轴 2–16，`Origin`、`Step` 定义均匀三维网格。`Lights` 为线性 RGB 点光源，按半径截断后以 `(1-d/r)^2/(1+d^2)` 衰减，再乘光源功率；能量以六轴的非负余弦投影，光源与探针重合时六个方向取相同的能量（每方向权重为 1）。`Blocks` 是轴对齐闭区间盒，探针在盒内或边界标为无效；射线与盒相切也阻挡。

`Samples` 的查询结果包含 `sampled`、`outside`、`solid` 或 `unsupported`。三线性权重只从有效且视线畅通的邻点归一化；`Weight` 是归一化前的支持率。JSON 中是线性域参考值；不包含间接反弹或材质模拟。

ZIP 包含 `layout.json` 和 `probes.png`。栅格坐标 `(x,y,z)` 与方向 `face` 的像素为 `(x+face*X,y+z*Y)`。有效像素 Alpha 为 255，即使其 RGB 全零；无效像素透明。RGB 使用 `round(255*(1-exp(-energy*Exposure)))` 映射，不是 sRGB 纹理。消费方应使用线性纹理采样并按 Alpha 判断有效性。

## HTTP 接口

| 入口 | 结果 |
| --- | --- |
| `GET /healthz` | 进程就绪 |
| `GET /v1/example` | 无外部依赖的合成请求 |
| `POST /v1/build` | 新成品返回 201；已存在的相同制品返回 200；均带 `digest`、`bytes`、`artifact` 与 `Location` |
| `GET /v1/artifacts/{digest}` | 原始成品下载；支持 ETag 与字节范围；未知摘要 404，摘要不符 409 |

提交仅接受 JSON，最多 3 MiB；未知字段、多余 JSON 值 400，类型错误 415，超体积 413，领域拒绝 422，同时处理中的构建最多 2 项，繁忙返回 503。不提供鉴权和多租户隔离，部署为本地工具服务，不应直接暴露到公网。应由使用者管理归档目录容量和保留期；服务不覆盖已有摘要。归档文件系统须支持硬链接。

## 单一职责

`cmd/server` 管理配置及监听，`internal/service` 负责请求和响应，`internal/domain` 拥有领域校验、核心计算和编码，`internal/vault` 负责归档和摘要完整性。中间结果只在一次构建中有效，成功成品按内容寻址，重启后仍可通过原摘要读取。
