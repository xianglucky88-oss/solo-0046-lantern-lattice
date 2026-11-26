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

## 时变烘焙

请求可选携带 `Animation`：给出动画 `Range`、落在其中的预览 `Shutter`，以及若干 `Movers`。每个移动盒尺寸固定，关键时刻严格递增并覆盖整段动画，关键时刻之间匀速平移。网格、点灯、静态盒与取样点保持固定。

时变成品 ZIP 在 `layout.json`、`probes.png` 之外新增 `timeline.json`：全局时间表加每个探针的分段恒定六向线性色。配合布局中的静态盒与移动盒，消费方可离线更换快门或取样位置，按同一时间平均定义复现支持率、状态与线性色，无需原始点灯清单或重新烘焙。未携带 `Animation` 的旧场景仍输出原来的双文件 v1 成品。

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、并发去重、重启复取和损坏拒绝，并以独立实现的离线引擎核对时变成品（换快门、换取样位置、相切零时长、全黑有效像素与逐像素曝光编码），随后关闭实例。可用 `product | persistence | contract | animation` 选择单项。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
