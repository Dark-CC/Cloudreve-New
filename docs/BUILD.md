# 构建与发布

## 依赖

- Go 1.26+（`go.mod` 声明 `go 1.26.0`）
- Node.js 20+，包管理器 pnpm 9（`web/pnpm-lock.yaml`）
- `zip`（打包静态资源）

## 1. 构建前端

```bash
cd web
pnpm install
npm_package_name=cloudreve-frontend npm_package_version=4.19.1 pnpm build
```

产品在 `web/build/`，其中 `version.json` 应为：

```json
{"name":"cloudreve-frontend","version":"4.19.1"}
```

## 2. 打包静态资源

服务端用 `fs.Sub(statics, "assets/build")` 读取，所以 zip 内**必须**保留
`assets/build/` 前缀：

```bash
cd web
rm -f ../src/application/statics/assets.zip
zip -r - assets/build > ../src/application/statics/assets.zip
```

压缩后的结构：

```text
assets/build/index.html
assets/build/version.json
assets/build/assets/...
```

直接压缩 `build/*` 会让首页返回 404。

## 3. 构建后端

```bash
cd src
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w \
    -X 'github.com/cloudreve/Cloudreve/v4/application/constants.BackendVersion=4.19.1' \
    -X 'github.com/cloudreve/Cloudreve/v4/application/constants.LastCommit=$(git rev-parse --short HEAD)' \
    -X 'github.com/cloudreve/Cloudreve/v4/application/constants.IsPro=false'" \
  -o ../cloudreve .
```

交叉编译只需设置 `GOOS` / `GOARCH`，无需 CGO。

## 4. 发布检查

1. `./cloudreve --version` 版本号与 `version.json` 一致；
2. 首页可打开，浏览器控制台无 404；
3. 启动日志无 `Static resource version mismatch`；
4. 二进制与源码 tag 对应，GPL-3.0 要求同时提供源码。

## 关于 goreleaser

`src/.goreleaser.yaml` 保留了上游的构建矩阵。若要在自己的仓库发布，把
`image_templates`、`archives.name_template` 中的镜像/包名改成自己的命名空间即可。
