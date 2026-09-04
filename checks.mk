.PHONY: checks
checks: lint go-fix ## Run all code checks (lint + go fix)

#########################
# Lint
#########################

# golangci-lint and go fix both only ever see a single module, so they have to
# be run once per module.
#
# Modules are discovered through git rather than a filesystem walk: a plain
# `find` also descends into untracked scratch directories and into nested git
# worktrees checked out under the repo (e.g. .claude/worktrees/<branch>), and
# would lint code that is not part of this checkout. That needed an
# ever-growing list of -not -path exclusions; git needs none.
#
# tools/ is excluded on purpose: it is a build-tagged dependency pin with no
# buildable package, and golangci-lint fails there with "no go files to
# analyze".
GO_MODULE_DIRS = $(shell cd $(TOP) && git ls-files \
	| grep -E '(^|/)go\.mod$$' \
	| xargs -n1 dirname \
	| grep -vx tools \
	| sort)

# run a command in every module, reporting every module that fails rather
# than stopping at the first
define in_all_modules
	rc=0; \
	for dir in $(GO_MODULE_DIRS); do \
		echo "  -> $$dir"; \
		(cd $$dir && $(1)) || rc=1; \
	done; \
	exit $$rc
endef

.PHONY: list-go-modules
list-go-modules: ## List the go modules covered by `make checks`
	@$(foreach dir,$(GO_MODULE_DIRS),echo "$(dir)";)

# Deliberately not --new-from-rev: that reports only issues attributable to
# changed lines, so it misses deletions (removing a license header adds no
# lines), misses issues on untouched lines of touched files, and disagrees with
# what CI enforces. It also saves no time, since golangci-lint analyses
# everything either way and only filters the report.
.PHONY: lint
lint: ## Run linter
	@echo "Running Go Linters..."
	@$(call in_all_modules,golangci-lint run --color=always --timeout=4m)

.PHONY: lint-auto-fix
lint-auto-fix: ## Run linter with auto-fix
	@echo "Running Go Linters with auto-fix..."
	@$(call in_all_modules,golangci-lint run --color=always --timeout=4m --fix)

.PHONY: lint-fmt
lint-fmt:
	@echo "Running Go Formatters..."
	@$(call in_all_modules,golangci-lint fmt)

#########################
# Go fix
#########################

# `go-fix` only reports, which keeps `checks` read-only like `lint`.
# `go-fix-apply` is the same analysis, rewriting the code in place.
.PHONY: go-fix
go-fix: ## Report go fix modernizations (dry run)
	@echo "Running go fix..."
	@$(call in_all_modules,go fix -diff ./...)

.PHONY: go-fix-apply
go-fix-apply: ## Apply go fix modernizations in every module
	@echo "Applying go fix..."
	@$(call in_all_modules,go fix ./...)
