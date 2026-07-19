#!/bin/sh
# wt worktree.post_create hook — auto-detect project types and install deps.
# Runs in the newly created worktree (WT_WORKTREE_PATH) after `wt new`.
# Edit freely; manage with `wt hook list|disable|remove`.

set -eu

ROOT="${WT_WORKTREE_PATH:-$(pwd)}"
echo "🛠  wt post-create hook running in: $ROOT"

# JS runtimes / package managers by lockfile priority.
# Frozen modes never touch the lockfile; if the lockfile is out of sync
# with the manifest we warn and fall back to a normal install so one
# stale lockfile doesn't block the rest of the worktree setup.
install_js() {
  cd "$1"
  if [ -f "pnpm-lock.yaml" ] && command -v pnpm >/dev/null 2>&1; then
    echo "📦  [js] pnpm install --frozen-lockfile (pnpm-lock.yaml found)"
    pnpm install --frozen-lockfile || {
      echo "⚠️  [js] pnpm-lock.yaml out of sync with package.json; falling back to pnpm install"
      pnpm install
    }
  elif [ -f "yarn.lock" ] && command -v yarn >/dev/null 2>&1; then
    # Yarn 2+ (Berry) uses --immutable; Yarn 1 uses --frozen-lockfile.
    if [ -f ".yarnrc.yml" ]; then
      echo "📦  [js] yarn install --immutable (yarn.lock + .yarnrc.yml found)"
      yarn install --immutable || {
        echo "⚠️  [js] yarn.lock out of sync with package.json; falling back to yarn install"
        yarn install
      }
    else
      echo "📦  [js] yarn install --frozen-lockfile (yarn.lock found)"
      yarn install --frozen-lockfile || {
        echo "⚠️  [js] yarn.lock out of sync with package.json; falling back to yarn install"
        yarn install
      }
    fi
  elif [ -f "bun.lockb" ] && command -v bun >/dev/null 2>&1; then
    echo "📦  [js] bun install --frozen-lockfile (bun.lockb found)"
    bun install --frozen-lockfile || {
      echo "⚠️  [js] bun.lockb out of sync with package.json; falling back to bun install"
      bun install
    }
  elif [ -f "package-lock.json" ] && command -v npm >/dev/null 2>&1; then
    echo "📦  [js] npm ci (package-lock.json found)"
    npm ci
  elif command -v pnpm >/dev/null 2>&1; then
    echo "📦  [js] pnpm install (no lockfile)"
    pnpm install
  elif command -v npm >/dev/null 2>&1; then
    echo "📦  [js] npm install (no lockfile)"
    npm install
  else
    echo "⚠️  [js] package.json found but no pnpm/yarn/bun/npm available"
  fi
}

# Python environments by lockfile / manifest priority.
install_python() {
  cd "$1"
  if [ -f "poetry.lock" ] && command -v poetry >/dev/null 2>&1; then
    # poetry install is strict when poetry.lock exists; it never updates it.
    echo "🐍  [py] poetry install (poetry.lock found)"
    poetry install
  elif [ -f "uv.lock" ] && command -v uv >/dev/null 2>&1; then
    # --locked: fail if uv.lock is out of sync with pyproject.toml.
    echo "🐍  [py] uv sync --locked (uv.lock found)"
    uv sync --locked || {
      echo "⚠️  [py] uv.lock out of sync with pyproject.toml; falling back to uv sync"
      uv sync
    }
  elif [ -f "Pipfile.lock" ] && command -v pipenv >/dev/null 2>&1; then
    # --deploy: fail if Pipfile.lock is out of date; never regenerates it.
    echo "🐍  [py] pipenv install --dev --deploy (Pipfile.lock found)"
    pipenv install --dev --deploy || {
      echo "⚠️  [py] Pipfile.lock out of sync with Pipfile; falling back to pipenv install --dev"
      pipenv install --dev
    }
  else
    if [ ! -d ".venv" ]; then
      echo "🐍  [py] creating .venv"
      python3 -m venv .venv
    fi
    echo "🐍  [py] installing requirements into .venv"
    .venv/bin/python -m pip install --upgrade pip
    if [ -f "requirements.txt" ]; then
      .venv/bin/pip install -r requirements.txt
    fi
  fi
}

# Recursively scan the worktree for project roots (node_modules excluded).
scan_projects() {
  search_root="$1"

  # Python first so requirements.txt dirs don't shadow more specific tools.
  find "$search_root" -maxdepth 3 \( \
    -name "requirements.txt" -o \
    -name "pyproject.toml" -o \
    -name "Pipfile.lock" -o \
    -name "uv.lock" -o \
    -name "poetry.lock" \
    \) -print 2>/dev/null | while IFS= read -r marker; do
    install_python "$(dirname "$marker")"
  done

  # JS/Node.
  find "$search_root" -maxdepth 3 -name "package.json" \
    -not -path "*/node_modules/*" -print 2>/dev/null | while IFS= read -r marker; do
    install_js "$(dirname "$marker")"
  done

  # Go. `go mod download` never modifies go.mod/go.sum.
  find "$search_root" -maxdepth 3 -name "go.mod" -print 2>/dev/null | while IFS= read -r marker; do
    echo "🐹  [go] go mod download ($marker)"
    (cd "$(dirname "$marker")" && go mod download)
  done

  # Rust. --locked asserts Cargo.lock is up to date; fall back if stale.
  find "$search_root" -maxdepth 3 -name "Cargo.toml" -print 2>/dev/null | while IFS= read -r marker; do
    echo "🦀  [rust] cargo fetch --locked ($marker)"
    (cd "$(dirname "$marker")" && cargo fetch --locked) || {
      echo "⚠️  [rust] Cargo.lock out of sync with Cargo.toml; falling back to cargo fetch"
      (cd "$(dirname "$marker")" && cargo fetch)
    }
  done
}

scan_projects "$ROOT"

echo "✅  wt post-create hook finished"
