# V2 验证命令与环境

2026-10-31，当前托管 Linux 工作区（非用户目标 2 核/8GB 生产机）：Linux 6.18.57 / amd64，cgroup 4 CPU、32GiB RAM，32GB overlay 文件系统，Docker 28.4.0 / vfs。Go 1.25.0、Node 24.19.0、pnpm 10.24.0、Python 3.12；数据库为隔离 Docker PostgreSQL 17.11 和 ClickHouse 25.10.2.65。没有发布公网服务、更新生产数据或调用真实收费模型。

数据库运行命令（test-only credential，可自行替换）：

```sh
docker run -d --name catena-v2-pg -e POSTGRES_USER=catena_test -e POSTGRES_PASSWORD=catena-v2-test-only -e POSTGRES_DB=catena_test -p 127.0.0.1:55432:5432 postgres:17-alpine
docker run -d --name catena-v2-ch -e CLICKHOUSE_USER=default -e CLICKHOUSE_PASSWORD=catena-v2-test-only -e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 -p 127.0.0.1:58123:8123 clickhouse/clickhouse-server:25.10.2.65-alpine
python3.12 -m venv /tmp/catena-v2-clean
/tmp/catena-v2-clean/bin/pip install -e 'engine[dev,rsi]'
/tmp/catena-v2-clean/bin/pip check
cd engine
/tmp/catena-v2-clean/bin/ruff check catena_engine tests
/tmp/catena-v2-clean/bin/python -m pytest -o addopts='' -q
cd ../control-plane
BARENA_TEST_DATABASE_URL='postgres://catena_test:catena-v2-test-only@127.0.0.1:55432/catena_test?sslmode=disable' CATENA_CLICKHOUSE_TEST_DSN='http://default:catena-v2-test-only@127.0.0.1:58123/default?dial_timeout=5s' CATENA_ENGINE_TEST_PYTHON=/tmp/catena-v2-clean/bin/python go test -race -count=1 -timeout 3m ./...
go vet ./...
cd ../catena-web
pnpm test
pnpm build
CATENA_BROWSER_EXECUTABLE=/usr/bin/chromium pnpm test:browser
# 在 Go 编译前串行更新已生成的 embedded Web；禁止边替换 embed 目录边 Go 编译。
diff -qr dist ../control-plane/internal/control/web
cd ..
/tmp/catena-v2-clean/bin/pip install -e 'tap[dev]'
cd tap && /tmp/catena-v2-clean/bin/python -m pytest -o addopts='' -q
cd .. && python evals/trace-skill-benchmark/validate_verifiers.py
python scripts/source_inventory.py
```

实际 Go 可执行位于 /tmp/go/bin/go，pnpm 位于 /tmp/catena-v2-tools/node_modules/.bin/pnpm。Go 普通单元测试（无 DB env）、带真实隔离数据库的 race 套件均通过，原始终端输出见同目录 logs。pytest 的 deterministic Harness²/Harbor 测试使用 fixture 模型/验证器，不是付费模型成绩；跨进程测试的 OpenViking HTTP 端也是合成 fixture。

Compose 验证执行了 base、public、rsi、openviking 四种 config；public 使用 CI 中的非敏感 test-only 参数，RSI 使用 CATENA_RSI_WORKSPACE=/tmp/catena-rsi / CATENA_DOCKER_GID=998，Memory 使用 CATENA_OPENVIKING_API_KEY=fixture-only。均不执行 up、不启动提供方模型。Engine/core 全镜像与目标机器整栈启动未在本工作区实测；CI 增加了 Engine 镜像构建/版本/私有鉴权 smoke。
