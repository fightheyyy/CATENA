# 开发与维护

先读 [当前规格](SPEC.md)、[架构](docs/ARCHITECTURE.md) 和 [Workflow](docs/IMPROVEMENT_WORKFLOW.md)。

## 本地开发

Go 1.25、Python 3.12、Node.js 24、pnpm 10.24；服务配置见 [部署指南](deploy/catena-mvp1/README.md)。

```sh
python3.12 -m venv .venv
.venv/bin/python -m pip install -e 'engine[dev,evaluation]'
(cd frontend && pnpm install --frozen-lockfile --ignore-workspace)
```

Engine 基础安装不需要 Harbor；只做分析可省略 `evaluation` extra。真实运行还需要固定的 Codex 二进制与私有模型配置，部署镜像已固定其版本。

前端开发用 `(cd frontend && pnpm dev)`；Vite 将 API 代理到 `127.0.0.1:8787`，需先启动同地址的 Go 服务。API 密钥保存在本机受限配置或 Connections 中，不放进 Git、截图或测试产物。

## 按改动验证

```sh
# Go
(cd backend && go test ./... && go vet ./...)

# Engine
(cd engine && ../.venv/bin/ruff check catena_engine tests && ../.venv/bin/python -m pytest)

# Web
(cd frontend && pnpm test && pnpm build && pnpm test:browser)
```

浏览器测试首次需安装 Chromium：`(cd frontend && pnpm exec playwright install chromium)`。
数据库、跨进程、race、Tap 和 Compose 检查由 [CI](.github/workflows/catena-mvp1-ci.yml) 定义；本机缺少服务时明确记录未执行项。真实模型 / Docker E2E 单独记录配置、Case 指纹、Verifier、轨迹、用量和人工决定。

只为有风险的新逻辑补小而有效的测试，优先复用已有测试。文档整理检查链接即可，不为文案新增单元测试。

## 前端构建产物

Go 嵌入 `backend/internal/control/web/`。前端变更提交前构建并同步，避免源码已更新而部署仍显示旧页面：

```sh
(cd frontend && pnpm build)
rm -rf backend/internal/control/web/assets
cp -R frontend/dist/. backend/internal/control/web/
diff -qr frontend/dist backend/internal/control/web
```

只替换上述生成目录，不手写 bundle。CI 会检查源码构建和嵌入目录一致。

## 提交与分支

- `main` 是集成入口；一个改动使用一个短期 `codex/<topic>` 分支，完成后合并、删除已合并分支。
- 合并前检查工作区、差异和相关验证。推送使用普通快进或 PR，不强推覆盖 `main`。
- 删除分支前验证其提交已进入 `main`；存在独立提交、打开的 PR 或未提交改动时保留。
- 不重置他人的工作区，不删除本地实验目录、数据库卷或重要历史。数据库迁移追加，不改写已应用迁移。
- 文档职责唯一：根 README 负责入口，SPEC 负责范围，架构说明模块，Workflow 负责执行契约，部署目录负责配置。旧方案进入 `docs/archive/`，实验和迁移证据保留原始记录。

## 变更边界

不新增第二套 Agent Runtime、Runner、Verifier、Memory Framework 或没有现实需求的通用插件层。
分析结果必须区分观察、假设和不确定性；生成候选前人工决定，采用前独立评测并人工 Review。
