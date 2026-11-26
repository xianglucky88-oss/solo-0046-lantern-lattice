# 灯格 · 遮挡探针烘焙

面向静态场景照明烘焙师。输入三维均匀测点网格、点光源、遮挡盒和采样坐标，输出六方向 RGB 探针 PNG 条带、有效位与空间采样数据。仅提供 HTTP API，不包含前端，不依赖第三方 Go 模块。

## 运行

需要 Go 1.22 或更高版本。在本仓根目录启动：

```sh
go run ./cmd/server
```

默认监听 `127.0.0.1:8752`，归档目录默认 `.local/`。可指定 `-port` 与 `-data`，例如 `go run ./cmd/server -port 8752 -data .local`。

```sh
curl -fsS http://127.0.0.1:8752/v1/example -o request.json
curl -fsS -H 'Content-Type: application/json' --data-binary @request.json http://127.0.0.1:8752/v1/build
```

取返回 JSON 里的 `artifact` 路径，向同一服务发送 GET，获得 `probe-volume.zip`。输出摘要是交付原始字节的 SHA-256。

## 设备纹理上限与离线导入

只能上传小纹理的设备可在请求中同时给出 `MaxTextureWidth` 与 `MaxTextureHeight`（均为至少 2 的整数）。烘焙改产出 `lantern-probes/v2` 分页包：`layout.json` 声明每张 `pages/*.png` 的实际尺寸，以及每个矩形片段对应的方向（`Face`）、深度层（`Depth`）、原纹理面内区域（`Source`）和页内区域（`Target`）。每页都能单独上传且不超过上限；片段在需要时重叠切分，保证不产生小于 2×2 的页。缺省两个上限时仍是旧版 `probes.png` 单页成品。

```sh
curl -fsS -H 'Content-Type: application/json' \
  -d '{"Grid":{"Origin":[0,0,0],"Step":1,"Size":[3,2,2]},"Exposure":3,
       "Lights":[],"Blocks":[],"Samples":[],
       "MaxTextureWidth":64,"MaxTextureHeight":64}' \
  http://127.0.0.1:8752/v1/build
```

两种包走同一个离线导入入口；缺少声明页、片段越界、逻辑覆盖空洞或重复像素 RGBA 冲突都在导入阶段拒绝，即使该位置还没有被查询：

```sh
curl -fsS -H 'Content-Type: application/zip' --data-binary @probe-volume.zip \
  http://127.0.0.1:8752/v1/import
```

导入后按方向与深度层取样，`u`、`v` 为 `[0,1]` 面内小数坐标，只使用该方向和层的逻辑双线性足迹。编码 RGB 按线性通道加权平均，响应同时给出有效权重之和；有效黑参与权重，透明点不参与，权重为零时 `Available` 为 `false`：

```sh
curl -fsS 'http://127.0.0.1:8752/v1/artifacts/<digest>/sample?face=0&depth=0&u=0.25&v=0.75'
```

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、分页烘焙、离线导入拒绝条件、跨页采样一致性、并发去重、重启复取和损坏拒绝，随后关闭实例。也可单独运行 `product`、`contract`、`tiled` 或 `persistence` 模式。Go 单元测试用 `go test ./...`。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
