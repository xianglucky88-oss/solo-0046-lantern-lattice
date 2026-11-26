# 灯格 · 遮挡探针烘焙

面向静态场景照明烘焙师。输入三维均匀测点网格、点光源、遮挡盒和采样坐标，输出六方向 RGB 探针 PNG 条带、有效位与空间采样数据。支持在现有实心盒上指定至多 16 个可换色反射面，烘焙一次后仅凭下载 ZIP 离线按面 ID 反复改色。仅提供 HTTP API 与离线预览工具，不依赖第三方 Go 模块。

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

## 反射面与离线改色

在请求中加入 `Faces`（≤16），每项给稳定 `ID`、面中心 `Center`、外法线 `Normal` 与初始线性 RGB `Reflectance`（0–1）。探针能量 = 原灯直射 + 经一个指定面 + 依次经两个指定面的光；公式、闭盒遮挡的端点盒面豁免、ZIP 内 `Preview` 基格式与离线 CLI 详见 [README.md](README.md) 的“可换色反射面”。

```sh
curl -fsS -H 'Content-Type: application/json' --data-binary @examples/corner.json \
  http://127.0.0.1:8752/v1/build     # 两面墙拐角示例
python3 -I tools/preview.py probe-volume.zip --set wall_a=0,1,1 -o out/
```

## 核验

`go build ./...` 构建全部 Go 包，`go test ./...` 运行领域公式与 JSON 兼容单测。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、并发去重、重启复取和损坏拒绝，随后关闭实例。`python3 -B tools/check_reflection.py` 额外验证两面墙反射、离线改色、像素/线性一致性与通道消除。完整测试套件暂缓。详见 [README.md](README.md)。
