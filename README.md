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

## 探针救援

请求体可选 `Rescue` 段，用于在同一次烘焙中挪动被遮挡盒压住的探针：`MaxMove` 限定最大挪动距离，`Slots` 最多 8 个待救槽（每槽最多 4 个候选落点），`Checkpoints` 指定要保住的检查点和各自的最低支持率。候选须在原网格范围内、不超过最大挪动距离并避开实心盒；服务按“达标检查点最多、平局总挪动最短”确定性地为每槽选择一个落点或保留无效。

移位探针按 `1 - move/MaxMove` 线性折减，支持率与线性照明都使用折减后的权重，视线仍被遮挡盒（含相切）阻断。成品 `layout.json` 的 `Rescue` 字段记录各槽实际落点、挪动距离、折减因子以及每个检查点的达标情况、支持率和线性色；`probes.png` 仍按原槽位寻址。

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、并发去重、重启复取和损坏拒绝，随后关闭实例。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
