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

## 六轴天光与预览改灯

请求可附带 `Sky`：与 `Axes` 对齐的六个线性 RGB 颜色（`+X,-X,+Y,-Y,+Z,-Z`）。每个有效探针沿对应轴射出无限远射线；任一闭盒（相切也算）挡住该方向时，该轴天光为零，否则取该轴颜色。点光源规则不变；普通烘焙的 `Samples.Irradiance` 与 `probes.png` 都使用“点灯 + 天光”的原始亮度。

带 `Sky` 的成品会在 `layout.json` 内嵌自包含的线性分组基（顶层 `Probes[].Light/.Sky/.Irradiance` 与 `Preview`），因此下载一次 ZIP 后即可脱离原请求反复改灯：

```sh
# HTTP：倍率 0..1，缺省为 1；POST 返回含 base64 PNG 与线性取样的 JSON
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"SkyScale":0.25,"LightScale":1,"Positions":[[0.4,0,0]]}' \
  http://127.0.0.1:8752/v1/artifacts/<digest>/preview
# GET 同名入口（或 /relight、/texture、/probes.png）默认直接返回改灯后的 PNG
curl -s "http://127.0.0.1:8752/v1/artifacts/<digest>/preview?sky=0&lights=1" -o lit.png
# 任意原烘焙没问过的体内位置取线性色
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"Positions":[[0.5,0.5,0.5]],"SkyScale":0,"LightScale":0}' \
  http://127.0.0.1:8752/v1/artifacts/<digest>/sample
```

倍率始终作用于内嵌的原始线性分组：`L = lightScale*Light + skyScale*Sky`，随后才按原 `Exposure` 曲线量化为贴图，连续调节不会叠加前一次的量化误差。空间取样沿用原可见邻点与归一化支持率；两组倍率都为 0 时，有支持的点仍是 `State:"sampled"`、`Weight>0` 的有效黑。同样的操作也可用纯标准库离线工具完成：

```sh
python3 -I tools/preview.py probe-volume.zip --sky 0.25 --lights 1 \
  --at 0.4,0,0 -o out/      # 另见根目录 preview.py、tools/relight.py 与 go run ./cmd/preview
```

不带 `Sky` 的旧请求不输出扩展字段，ZIP 字节与旧版完全一致，继续交付原成品。

## 核验

`go build ./...` 构建全部 Go 包。安装 Python 3.9+ 后运行 `python3 -B tools/verify.py all`：启动临时回环服务，用内置样例验证实际产物与拒绝边界、并发去重、重启复取和损坏拒绝，随后关闭实例。完整测试套件暂缓。详见 [PROJECT_SPEC.md](PROJECT_SPEC.md)。
