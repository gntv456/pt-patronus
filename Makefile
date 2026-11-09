.PHONY: help dev-backend dev-frontend dev-mobile test test-backend test-frontend test-mobile image clean embed hooks perf perf-check perf-baseline-init gen-site-index analyze lint-ep

help:
	@echo "PTPatronus 开发/构建命令"
	@echo "  make dev-backend    启动后端(默认:8088, SQLite)"
	@echo "  make dev-frontend   启动 Web UI(:5173, 代理 /api)"
	@echo "  make dev-mobile     启动 Flutter 客户端"
	@echo "  make test           跑全部测试(后端+前端+客户端)"
	@echo "  make image          构建单镜像(API + 内嵌 Web UI)"
	@echo "  make up             docker compose 启动"
	@echo "  make embed-preview  把前端产物拷进后端 web/dist 便于本地 go run 验证内嵌"
	@echo "  make hooks          启用 .githooks（pre-push 对改动的后端包跑 go vet+test，旁路 --no-verify）"
	@echo "  make perf           采集性能基线（可选 PERF_URL=http://127.0.0.1:8088）"
	@echo "  make perf-check     性能棘轮门禁"
	@echo "  make perf-baseline-init  把本次采集固化为新基线"

dev-backend:
	cd backend && DEFINITIONS_DIR=./definitions go run ./cmd/ptpatronus

dev-frontend:
	cd frontend && npm run dev

dev-mobile:
	cd mobile && flutter run

test: test-backend test-frontend test-mobile

test-backend:
	cd backend && go vet ./... && go test ./...

# 启用共享 git 钩子（pre-push：对推送改动的后端 Go 包跑 go vet + go test -short）。
# 每个 clone 一次性；旁路用 git push --no-verify 或 PTPATRONUS_SKIP_TESTS=1。
hooks:
	git config core.hooksPath .githooks
	@echo "✓ core.hooksPath=.githooks（pre-push 生效）"

test-frontend:
	# 实跑单测 + 严格类型检查（曾只跑 build——名为测试实为构建，测试审计 P3-2）
	cd frontend && npm test && npm run typecheck

test-mobile:
	cd mobile && flutter analyze && flutter test

image:
	docker build -t ptpatronus:latest .

up:
	docker compose up -d --build

embed-preview:
	cd frontend && npm run build
	rm -rf backend/internal/web/dist
	cp -r frontend/dist backend/internal/web/dist
	@echo "前端产物已拷入 backend/internal/web/dist，可 go run 验证内嵌 Web UI"
	@bash scripts/check-no-gpl-assets.sh

# 性能门禁（棘轮）：见 docs/perf-baseline.md
perf:
	node scripts/collect-perf.mjs --url $(or $(PERF_URL),http://127.0.0.1:8088)

perf-check:
	node scripts/check_perf.mjs

analyze:
	node scripts/analyze-bundle.mjs

lint-ep:
	node scripts/check-no-new-ep.mjs

# 阶段 4：重新生成编译期站点索引（definitions/*.yaml 变更后跑）
gen-site-index:
	node scripts/gen-site-index.mjs

perf-baseline-init:
	node scripts/check_perf.mjs --init

clean:
	rm -rf backend/bin backend/data data frontend/dist
