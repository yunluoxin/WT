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

# has_js_deps reports whether package.json declares any third-party deps.
# Projects without dependencies (e.g. a bare Chrome extension) would get a
# useless empty node_modules/ and a newly created lockfile — skip those.
has_js_deps() {
  # No node to parse JSON? Assume deps exist (conservative: still install).
  command -v node >/dev/null 2>&1 || return 0
  node -e '
    const p = require("./package.json");
    const deps = ["dependencies", "devDependencies", "peerDependencies", "optionalDependencies"]
      .some((k) => p[k] && Object.keys(p[k]).length > 0);
    process.exit(deps ? 0 : 1);
  ' 2>/dev/null
}

install_js() {
  cd "$1"
  if ! has_js_deps; then
    echo "⏭️   [js] package.json has no dependencies; skipping install"
    return 0
  fi
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

  # Swift / iOS / macOS.
  #
  # SPM: resolve per Package.swift, skipping nested checkouts (.build) and
  # vendored sources (Sources). Package.resolved pins exact versions;
  # resolve never rewrites it unless requirements changed.
  find "$search_root" -maxdepth 4 -name "Package.swift" \
    -not -path "*/.build/*" -not -path "*/Sources/*" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    echo "🐦  [swift] swift package resolve ($dir)"
    (cd "$dir" && swift package resolve)
  done

  # Ruby. Runs BEFORE CocoaPods: a Gemfile that pins the cocoapods gem is
  # the standard iOS setup, and `bundle exec pod` below needs that gem
  # installed first. Skip Gemfiles that declare no gems (installing those
  # would create a lockfile the project never had). Otherwise `bundle check`
  # succeeds only if the lockfile's gems are already installed, avoiding
  # a redundant install on repeat worktree creation.
  find "$search_root" -maxdepth 3 -name "Gemfile" \
    -not -path "*/vendor/bundle/*" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if ! grep -Eq "^[[:space:]]*gem[[:space:]]" "$marker"; then
      echo "⏭️   [ruby] Gemfile declares no gems; skipping ($dir)"
      continue
    fi
    if command -v bundle >/dev/null 2>&1; then
      echo "💎  [ruby] bundle install ($dir)"
      (cd "$dir" && (bundle check || bundle install))
    else
      echo "⚠️  [ruby] Gemfile found but bundler not available"
    fi
  done

  # CocoaPods: only in dirs that actually have a Podfile with dependencies
  # (a bare `target` block produces no lockfile and just errors). --deployment
  # forbids Podfile.lock changes; fall back to plain install if stale.
  #
  # If the project also has a Gemfile declaring cocoapods (the common iOS
  # setup: Gemfile pins the pod version, Podfile the pods), run pod through
  # `bundle exec` so the pinned version is used instead of the PATH one.
  find "$search_root" -maxdepth 4 -name "Podfile" \
    -not -path "*/Pods/*" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if ! grep -Eq "^[[:space:]]*pod[[:space:]]" "$marker"; then
      echo "⏭️   [pods] Podfile declares no pods; skipping ($dir)"
      continue
    fi
    pod_cmd="pod"
    if [ -f "$dir/Gemfile" ] && grep -q "cocoapods" "$dir/Gemfile" && command -v bundle >/dev/null 2>&1; then
      pod_cmd="bundle exec pod"
      echo "🍎  [pods] Gemfile pins cocoapods; using bundle exec ($dir)"
    fi
    echo "🍎  [pods] $pod_cmd install --deployment ($dir)"
    (cd "$dir" && $pod_cmd install --deployment) || {
      echo "⚠️  [pods] Podfile.lock out of sync with Podfile; falling back to $pod_cmd install"
      (cd "$dir" && $pod_cmd install)
    }
  done

  # Gradle (Android / JVM). Prefer the project-local wrapper (./gradlew),
  # which pins the Gradle version; use system gradle only as a fallback.
  # `gradle help` resolves the build's plugins + dependencies without
  # compiling. Skip subprojects (they have no settings.gradle and would
  # resolve the whole build again).
  find "$search_root" -maxdepth 3 \( -name "build.gradle" -o -name "build.gradle.kts" \) \
    -not -path "*/build/*" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if [ ! -f "$dir/settings.gradle" ] && [ ! -f "$dir/settings.gradle.kts" ] && [ ! -x "$dir/gradlew" ]; then
      continue  # subproject; the root build covers it
    fi
    echo "🐘  [gradle] resolving dependencies ($dir)"
    if [ -x "$dir/gradlew" ]; then
      (cd "$dir" && ./gradlew help -q)
    elif command -v gradle >/dev/null 2>&1; then
      (cd "$dir" && gradle help -q)
    else
      echo "⚠️  [gradle] build.gradle found but no ./gradlew or gradle available"
    fi
  done

  # Flutter / Dart. Flutter projects have pubspec.yaml + pubspec.lock.
  find "$search_root" -maxdepth 3 -name "pubspec.yaml" \
    -not -path "*/.dart_tool/*" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if [ -f "$dir/pubspec.lock" ] && command -v flutter >/dev/null 2>&1; then
      echo "🎯  [flutter] flutter pub get ($dir)"
      (cd "$dir" && flutter pub get)
    elif command -v dart >/dev/null 2>&1; then
      echo "🎯  [dart] dart pub get ($dir)"
      (cd "$dir" && dart pub get)
    else
      echo "⚠️  [dart] pubspec.yaml found but no flutter/dart available"
    fi
  done

  # PHP. `composer install` is strict when composer.lock exists.
  find "$search_root" -maxdepth 3 -name "composer.json" \
    -not -path "*/vendor/*" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if command -v composer >/dev/null 2>&1; then
      echo "🐘  [php] composer install ($dir)"
      (cd "$dir" && composer install --no-interaction --prefer-dist)
    else
      echo "⚠️  [php] composer.json found but composer not available"
    fi
  done

  # .NET. Restore at the solution level when one exists (covers all
  # projects), otherwise per-project. --locked-mode asserts the lock file.
  find "$search_root" -maxdepth 3 \( -name "*.sln" -o -name "*.csproj" -o -name "*.fsproj" \) \
    -not -path "*/bin/*" -not -path "*/obj/*" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if command -v dotnet >/dev/null 2>&1; then
      echo "🔷  [.net] dotnet restore ($marker)"
      (cd "$dir" && dotnet restore "$marker" --locked-mode 2>/dev/null) || \
        (cd "$dir" && dotnet restore "$marker")
    else
      echo "⚠️  [.net] $marker found but dotnet not available"
    fi
  done

  # Java / Maven.
  find "$search_root" -maxdepth 3 -name "pom.xml" \
    -not -path "*/target/*" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if command -v mvn >/dev/null 2>&1; then
      echo "☕  [maven] dependency:go-offline ($dir)"
      (cd "$dir" && mvn -q dependency:go-offline -B)
    else
      echo "⚠️  [maven] pom.xml found but mvn not available"
    fi
  done
}

scan_projects "$ROOT"

echo "✅  wt post-create hook finished"
