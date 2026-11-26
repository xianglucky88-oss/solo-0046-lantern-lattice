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

## 续烘

初次烘焙请求带 `"Resumable": true`，回执即给出灯具版本身份（`rv_…`）和每盏灯的稳定标识。之后向 `POST /v1/revisions` 提交基版身份、重求额度和按稳定标识的 `add`/`remove`/`change` 操作，即可在保留网格、遮挡盒、查询点与曝光的前提下继续烘焙。回执的 `resolved` 是需要重求线性照明的有效探针坐标（受额度约束，超额 422），`occlusion_only` 是仅统计变化、不占额度的坐标；新成品仍从 `GET /v1/artifacts/{digest}` 下载，与按编辑后灯光全量烘焙的字节一致。

```sh
go build -o /tmp/offline ./cmd/offline
/tmp/offline -patch supplement.json -receipt previous-receipt.json -zip previous.zip -out probe-volume.zip
```

`GET /v1/patches/{version}` 取得补件后，`cmd/offline` 仅凭补件、旧版回执和旧包即可离线重建字节相同的新包；旧灯具版本身份或旧包摘要任一不符都会拒绝。

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、并发去重、重启复取和损坏拒绝，并覆盖续烘额度、仅遮挡探针、幂等重送、同 ZIP 多版本以及离线重建与拒绝，随后关闭实例。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
