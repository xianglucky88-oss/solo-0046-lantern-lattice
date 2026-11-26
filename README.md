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

## 粗档 / 精档（分级照明）

在烘焙请求里加入两个可选参数即可请求分级交付（字段名兼容 Pascal / camel / snake 与多种近义写法，也可放在 `Tiering` 或 `Coarse` 对象里）：

```jsonc
{
  "CoarseByteLimit": 4096,        // 粗档照明“解包后”字节上限
  "MaxLinearError": 0.02          // 每方向每通道允许的绝对线性误差（曝光前）
}
```

等价写法包括 `CoarseByteCap` / `CoarseBudgetBytes` / `ByteBudget` / `coarse_byte_budget`，以及 `CoarseMaxLinearError` / `LinearColorError` / `Tolerance` / `max_linear_error`。也支持 `POST /v1/build?coarseByteLimit=...&maxLinearError=...`。

构建结果在原有 `digest` / `bytes` / `artifact`（原成品，保持不变）之外，额外给出可分别下载的 `coarse` 与 `fine` 收据，各自有独立的摘要、字节数与 `/v1/artifacts/{digest}` 地址（支持 ETag / Range / 重启复取）：

```jsonc
{
  "digest": "<原始成品摘要>", "artifact": "/v1/artifacts/<原始摘要>",
  "coarse": {
    "digest": "<粗档摘要>", "bytes": 1595,
    "artifact": "/v1/artifacts/<粗档摘要>",
    "lightingBytes": 114,          // 计入预算的解包后照明字节
    "byteAccounting": {
      "ColorsBytes": 24,           // 颜色（调色板 / 量化通道）
      "CoefficientsBytes": 24,     // 系数（每 RGB 通道缩放）
      "LookupBytes": 66,           // 查找信息（调色板索引等）
      "TotalBytes": 114            // = 三者之和 = lightingBytes
    },
    "MaxLinearErrorObserved": 0.0184
  },
  "fine": { "digest": "<精档摘要>", "artifact": "/v1/artifacts/<精档摘要>" }
}
```

预算只统计粗档 zip 解包后真正用于还原照明的颜色、系数与查找数组；网格规格（`Grid`）和实心盒（`Blocks`）随粗档一起交付以便离线判定状态/视线，但不计入该上限。粗档会搜索直接量化与 RGB 调色板等表示，取能保证误差的最小解包体积；若仍超过 `CoarseByteLimit`，接口返回 `422` 与稳定错误码 `coarse_budget_infeasible`（含 `requiredBytes`）。若希望此时仍保留原成品与精档，可加 `"Partial": true`，改为 200/201 且 `coarse.available=false`。

不带任何分级参数时，响应与 zip 字节和旧版本完全一致。

### 仅用粗档做离线查询

`POST /v1/query`（别名 `/v1/sample`、`/v1/artifacts/{digest}/query` 等）只需粗档摘要即可查询烘焙 `Samples` 里没有出现过的任意位置，不需要灯具或精档：

```sh
curl -fsS -H 'Content-Type: application/json' \
  -d '{"coarse":"<粗档摘要>","positions":[[0.4,0,0],[1.5,0.5,0.5]]}' \
  http://127.0.0.1:8752/v1/query
```

返回的每个结果保持与原烘焙一致的 `State`（`outside` / `solid` / `unsupported` / `sampled`）、`Weight`（归一化前支持率）和曝光前 `Irradiance`，并额外给出 `Tier` 与 `Neighbors` / `RequiredFineProbes`。粗档保证：有支持时各方向各通道线性值在 `MaxLinearError` 内，参考为零的通道仍精确为零。

仓库附带纯标准库 Python 离线引擎 `tools/coarse_query.py`，只读取下载到的粗档 zip 即可在进程外复现定位、闭盒占用、切向阻挡射线、视线筛选三线性支持与归一化：

```sh
python3 -B tools/coarse_query.py coarse.zip positions.json
```

### 精档分批

精档是每个探针的原始 float64 六向线性值。可整包下载 `fine.artifact`，也可按探针分批取：

```sh
curl -fsS -H 'Content-Type: application/json' \
  -d '{"probes":[0,1,2]}' \
  http://127.0.0.1:8752/v1/artifacts/<精档摘要>/fine
curl -fsS http://127.0.0.1:8752/v1/artifacts/<精档摘要>/probes/7
```

查询时用 `fine` 指定精档摘要、用 `fineProbes` / `fineBatches` 表示已到达的探针（接受线性下标、`[x,y,z]` 或 `{X,Y,Z}`）。一次查询的全部有效（通过占用与视线筛选的）邻点都具备精档数据时，整次查询使用与原烘焙逐位一致的线性色（`Tier:"fine"`）；只到一部分时，整次查询仍用粗档（`Tier:"coarse"`），`State` 与 `Weight` 不随精档到达变化。

### 分级功能核验

```sh
python3 -B tools/verify_tiers.py
```

该脚本启动临时回环服务，校验字节预算恒等式、任意位置的状态/支持率/误差/零通道、薄墙红/蓝双房与无光黑房、精档全量精确与部分回退、分批接口、预算失败、重启复取，以及 `tools/coarse_query.py` 的纯离线结果。

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、并发去重、重启复取和损坏拒绝，随后关闭实例。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
