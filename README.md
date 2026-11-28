# 灯格 · 遮挡探针烘焙

面向静态场景照明烘焙师。一次烘焙可为分处一张无厚度可倾斜薄墙两侧、间距不同的两套三维测点网格各输出六方向 RGB 探针 PNG 条带、有效位与空间采样数据，并能用同一世界坐标跨门取光。输入三维均匀测点网格、点光源、遮挡盒和采样坐标；仅提供 HTTP API，不包含前端，不依赖第三方 Go 模块。

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

取返回 JSON 里的 `artifact` 路径，向同一服务发送 GET，获得 `probe-volume.zip`。压缩包内 `layout.json` 描述场景与采样，`probes-0.png`、`probes-1.png` 分别是两套网格的六方向贴图。输出摘要是交付原始字节的 SHA-256。

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、并发去重、重启复取和损坏拒绝，随后关闭实例。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
