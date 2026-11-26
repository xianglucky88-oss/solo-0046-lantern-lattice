# 灯格：后端契约

## 领域流程

`Grid.Size` 每轴 2–16，`Origin`、`Step` 定义均匀三维网格。`Lights` 为线性 RGB 点光源，按半径截断后以 `(1-d/r)^2/(1+d^2)` 衰减，再乘光源功率；能量以六轴的非负余弦投影，光源与探针重合时六个方向取相同的能量（每方向权重为 1）。`Blocks` 是轴对齐闭区间盒，探针在盒内或边界标为无效；射线与盒相切也阻挡。

`Samples` 的查询结果包含 `sampled`、`outside`、`solid` 或 `unsupported`。三线性权重只从有效且视线畅通的邻点归一化；`Weight` 是归一化前的支持率。JSON 中是线性域参考值。

## 可换色反射面（一次烘焙，离线改色）

请求可额外提供至多 16 个 `Faces`，每个面是某个现有 `Blocks` 实心盒的一个面，用面中心 `Center`、单位外法线 `Normal` 代表整面，并带稳定标识 `ID` 与初始线性 RGB 漫反射率 `Reflectance`（每通道 `0..1`）：

```json
"Faces": [
  {"ID": "wall_a", "Center": [0, 2, 0], "Normal": [1, 0, 0],
   "Reflectance": [0.9, 0.9, 0.9]}
]
```

`ID` 必须非空且唯一；面必须落在某个闭盒面上（可给整数 `Box` 指定盒索引，否则按中心/法线自动匹配；给了 `Box` 时可省略 `Center`，由盒面中心推导）。字段同时接受常见别名：集合可用 `ReflectiveFaces`、`ColorableFaces`、`Reflectors` 等；标识可用 `Id/FaceID/Key`，反射率可用 `Albedo/Diffuse/Color`，中心可用 `Centroid/Position`，法线可用 `OutwardNormal`；PascalCase、camelCase、snake_case 均可。`Faces` 也支持 `{"id": {...}}` 映射形式，或嵌在 `Blocks[i]` 的 `Faces/ReflectiveFaces` 中。

在原有点光源直射之外，每个有效探针再累计：

- **一次反射** `灯 → 面 i → 探针`
- **两次反射** `灯 → 面 i → 面 j → 探针`（所有有序 `i,j`，路径顺序逐面乘 `Reflectance`，各 RGB 通道独立）

各段公式（所有余弦先取 `max(0, …)`）：

- 灯→面：原点灯同一 `attenuation(d, radius, power) = power·(1-d/r)^2/(1+d²)`（`d≥r` 为 0），乘该面入射余弦 `n_i·(L-C_i)/d`，再乘灯光 RGB。
- 面 i→面 j：`cosOut_i · cosIn_j / (1+d²)`，其中 `cosOut_i = n_i·(C_j-C_i)/d`、`cosIn_j = n_j·(C_i-C_j)/d`。
- 面 j→探针 p：`cosOut_j/(1+d²)`，`cosOut_j = n_j·(p-C_j)/d`，随后仍按原六向规则 `max(0, (C_j-p)·axis/d)` 投影。
- 路径颜色按通道乘反射率：一次乘 `rho_i`；两次依次乘 `rho_i·rho_j`。
- 任一反射段两端重合（`d=0`）时该段贡献为零。

每个反射段都对全部闭盒做遮挡检查。反射端点允许**单独**接触它所属的那个反射盒面（起点、终点各自豁免自己的盒面）；穿过盒体内部、接触任何其他盒/面仍判为遮挡。直射灯→探针与查询点→探针插值连线沿用原有严格闭盒规则，不享受端点豁免。查询状态、`Weight` 支持率与像素 Alpha 有效性只由原实心几何决定，与颜色无关。

## 离线预览工具

带 `Faces` 的成品，其 `layout.json` 额外内嵌一个 `Preview`（同时以 `Offline`、`Recolor`、`Basis` 键镜像）自包含基：每个探针的直射六向结果 `Direct`、灯→面 `LightFace`、面→面 `FaceFace`、面→探针六向权重 `FaceProbe`、初始 `Reflectance`，以及每个查询点冻结的探针链接 `Samples[].Links`。所有会触发射线遮挡的几何量都已冻结，离线改色只做线性重组，**不需要原始场景，也不重新做遮挡**。

仅用同一个下载包即可反复按 `ID` 改任意若干面的反射率；未指定的面（以及对象补丁中未指定的通道）用初始值；每次都从原始包重新出发，改动不叠加：

```sh
# 纯标准库 Python，可在隔离模式下运行
python3 -I tools/preview.py probe-volume.zip --set wall_a=0,1,1 -o out/
python3 -I tools/preview.py probe-volume.zip '{"wall_a":{"R":0}}'   # 只压红通道
python3 -I tools/preview.py probe-volume.zip changes.json --png probes.png --json s.json
```

兼容入口：`tools/recolor.py`、`tools/recolor_preview.py`、根目录 `preview.py`，以及 `go run ./cmd/preview`（别名 `./cmd/recolor`）。参数支持 `--set ID=R,G,B`（可重复，也可 `--set ID R G B`）、`--changes file.json`、位置参数里的 zip / JSON / `ID=R,G,B`。stdout 输出一份 JSON 报告，含每个原查询点的线性六色 `Samples`（及别名 `samples/Linear`）、探针线性 `Lobes` 和 `png_base64`；`-o DIR` 另写按原 `Exposure` 编码的 `probes.png`、`samples.json`、重算的 `layout.json` 与改色包。改色前后查询状态、支持率、像素有效性保持不变；线性查询色与 PNG 来自同一次“直射 + 反射”线性结果。

拐角探针若只收到 `灯→墙 A→墙 B→探针`，把任一面的红通道压为 0 都会令该光路红通道为 0，而未改动的绿通道保持通过。`examples/corner.json` 是可复现的两面墙拐角请求，`python3 -B tools/check_reflection.py` 覆盖公式、遮挡豁免、离线改色与通道消除。

不带 `Faces` 的旧请求不输出 `Preview` 段，ZIP 仍恰好是 `layout.json` 与 `probes.png`，交付字节与旧版一致。

## HTTP 接口

| 入口 | 结果 |
| --- | --- |
| `GET /healthz` | 进程就绪 |
| `GET /v1/example` | 无外部依赖的合成请求 |
| `POST /v1/build` | 新成品返回 201；已存在的相同制品返回 200；均带 `digest`、`bytes`、`artifact` 与 `Location` |
| `GET /v1/artifacts/{digest}` | 原始成品下载；支持 ETag 与字节范围；未知摘要 404，摘要不符 409 |

提交仅接受 JSON，最多 3 MiB；未知字段、多余 JSON 值 400，类型错误 415，超体积 413，领域拒绝 422，同时处理中的构建最多 2 项，繁忙返回 503。不提供鉴权和多租户隔离，部署为本地工具服务，不应直接暴露到公网。应由使用者管理归档目录容量和保留期；服务不覆盖已有摘要。归档文件系统须支持硬链接。

## 单一职责

`cmd/server` 管理配置及监听，`cmd/preview`、`cmd/recolor` 与 `tools/preview.py` 是只读下载包的离线改色预览入口，`internal/previewcli` 是 Go 预览 CLI 实现；`internal/service` 负责请求和响应，`internal/domain` 拥有领域校验、核心计算、编码与离线基重组，`internal/vault` 负责归档和摘要完整性。中间结果只在一次构建中有效，成功成品按内容寻址，重启后仍可通过原摘要读取。
