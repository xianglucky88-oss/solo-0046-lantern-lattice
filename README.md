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

`layout.json` 的 `Resource` 字段是精确的可实例化探针资源（`lantern-probe-resource/v1`），也可由 `domain.BuildResource` 单独生成，或用 `domain.ReadResourceArchive` 从成品 ZIP 读取。它保留展柜局部网格、灯参数、闭盒以及烘焙得到的灯到探针视线矩阵；引擎下载后无须再传原始灯具清单。实例查询示例：

```go
p := domain.Placement{
    Translation: domain.Vec{12, 0, 3},
    Scale:       0.25,                // 允许 0.25–4
    Axis:        [3]int{1, 0, 2},     // 每个世界轴读取的局部轴
    Flip:        [3]bool{false, false, false},
}
sample := resource.Sample(p, worldPoint)
```

世界坐标满足 `W[a] = Translation[a] + Scale*Sign[a]*Local[Axis[a]]`。查询点先逆变换到局部坐标，再沿用原网格边界容差、邻点权重、实心与视线规则；灯到探针遮挡使用烘焙矩阵，缩放后按世界距离和不变的世界单位射程重算衰减，并将六向色投影到世界轴。同一资源可并发服务不同缩放、轴交换和翻转的多个实例；不带实例参数的 `Build`/PNG/`Samples` 仍是原尺寸校样。

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、并发去重、重启复取和损坏拒绝，随后关闭实例。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
