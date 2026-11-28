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

## 可选：按接收面法线取光

请求可为每个 `Samples` 位置附带接收面法线（`Normals`，与 `Samples` 等长）。法线非零时按单位方向取半球内入射光，贡献乘入射余弦；半球边缘与背面为零，翻转法线即换半球。灯与探针重合时各法线等量取该灯。取样点不在探针中心时，沿用原有状态、可见邻点与支持率，由参与探针按同一法线加权供光，故与原六向结果在轴对齐法线上逐通道一致。

启用后成品 `layout.json` 追加 `NormalSamples` 与自包含 `NormalResource`（`lantern-normal-probes/v1`）。引擎只凭下载包、位置与法线即可离线取光：

```go
resource, err := domain.ReadNormalResource(zipBytes) // 或 layout.json 字节
sample := resource.Sample(domain.Vec{0.5, 0.5, 0.5}, domain.Vec{0, 0, 1})
```

也可用 CLI（仅需下载到的 ZIP）或 HTTP 入口，未启用新能力的成品会被拒绝：

```sh
/tmp/readprobe -artifact probe-volume.zip -point 0.5,0.5,0.5 -normal 0,0,1
curl -fsS -H 'Content-Type: application/json' -d '{"Digest":"<digest>","Position":[0.5,0.5,0.5],"Normal":[0,0,1]}' \
  http://127.0.0.1:8752/v1/normals/sample
```

未提供 `Normals` 时仍是原来的六方向成品与无方向取样，字节不变。

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、法线取光与离线读取、并发去重、重启复取和损坏拒绝，随后关闭实例。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
