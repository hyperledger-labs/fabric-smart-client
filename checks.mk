.PHONY: checks
checks: lint

#########################
# Lint
#########################

# golangci-lint only ever sees a single module, so it has to be run once per
# module. tools/ is excluded on purpose: it is a build-tagged dependency pin with
# no buildable package, and golangci-lint fails there with "no go files to
# analyze".
GO_MODULE_DIRS = $(shell find $(TOP) -name go.mod \
	-not -path '*/node_modules/*' \
	-not -path '$(TOP)/.claude/*' \
	-not -path '$(TOP)/tools/*' \
	-exec dirname {} \; | sort)

# run golangci-lint in every module, reporting every module that fails rather
# than stopping at the first
define lint_all_modules
	rc=0; \
	for dir in $(GO_MODULE_DIRS); do \
		echo "  -> $$dir"; \
		(cd $$dir && golangci-lint $(1)) || rc=1; \
	done; \
	exit $$rc
endef

.PHONY: list-go-modules
list-go-modules: ## List the go modules covered by `make lint`
	@$(foreach dir,$(GO_MODULE_DIRS),echo "$(dir)";)

# Deliberately not --new-from-rev: that reports only issues attributable to
# changed lines, so it misses deletions (removing a license header adds no
# lines), misses issues on untouched lines of touched files, and disagrees with
# what CI enforces. It also saves no time, since golangci-lint analyses
# everything either way and only filters the report.
.PHONY: lint
lint: ## Run linter
	@echo "Running Go Linters..."
	@$(call lint_all_modules,run --color=always --timeout=4m)

.PHONY: lint-auto-fix
lint-auto-fix: ## Run linter with auto-fix
	@echo "Running Go Linters with auto-fix..."
	@$(call lint_all_modules,run --color=always --timeout=4m --fix)

.PHONY: lint-fmt
lint-fmt:
	@echo "Running Go Formatters..."
	@$(call lint_all_modules,fmt)
