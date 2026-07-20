#!/bin/sh
# wt worktree.post_create hook — auto-detect project types and install deps.
# Runs in the newly created worktree (WT_WORKTREE_PATH) after `wt new`.
# Edit freely; manage with `wt hook list|disable|remove`.

set -u

ROOT="${WT_WORKTREE_PATH:-$(pwd)}"
echo "🛠  wt post-create hook running in: $ROOT"

# This hook is best-effort: one failing install must not abort the rest
# of the worktree setup. No `set -e` — failures are logged and we move on.
# (wt itself only warns on post-hook failure; the worktree is already
# created by the time we run.)
#
# SECURITY: install commands run dependency lifecycle scripts
# (postinstall/prepare). If you `wt new` a branch you do not trust, this
# hook executes that branch's code. Disable the hook for untrusted work.
WT_HOOK_ERR_FILE=""
trap 'rm -f "$WT_HOOK_ERR_FILE"' EXIT INT TERM

# JS runtimes / package managers by lockfile priority.
# Frozen modes never touch the lockfile; if the lockfile is out of sync
# with the manifest we warn and fall back to a normal install so one
# stale lockfile doesn't block the rest of the worktree setup.

# has_js_deps reports whether package.json declares any third-party deps.
# Projects without dependencies (e.g. a bare Chrome extension) would get a
# useless empty node_modules/ and a newly created lockfile — skip those.
# Parse failures (missing node, malformed JSON) are treated as "has deps":
# skipping an install on a parse error would silently leave a real
# project's dependencies uninstalled.
has_js_deps() {
  command -v node >/dev/null 2>&1 || return 0
  node -e '
    let p;
    try { p = require("./package.json"); } catch (e) { process.exit(0); }
    const deps = ["dependencies", "devDependencies", "peerDependencies", "optionalDependencies"]
      .some((k) => p[k] && Object.keys(p[k]).length > 0);
    process.exit(deps ? 0 : 1);
  ' 2>/dev/null
}

install_js() {
  # Absolute path: the marker walks below ascend parent dirs, and the
  # whole body runs in a subshell so its `cd` can't move the caller's cwd
  # (relative paths would break the walks and skip every other package).
  pkg_dir=$(cd "$1" && pwd)
  (cd "$pkg_dir" && _install_js_body "$pkg_dir")
  # The body reports the workspace root it installed via a
  # WT_JS_WS_ROOT: line on stderr (kept off stdout so captured output
  # stays clean); scan_projects records it so member packages of an
  # already-installed workspace can be skipped.
}

_install_js_body() {
  pkg_dir="$1"
  # Cap all upward walks at the worktree root: markers above $ROOT belong
  # to outer projects (a stray ~/pnpm-lock.yaml or a packageManager field
  # in ~) and must never steer detection or installs out of the worktree.
  walk_root="${WT_WORKTREE_PATH:-/}"
  case "$pkg_dir" in
    "$walk_root"/*|"$walk_root") ;;
    *) walk_root="/" ;; # called outside a worktree (tests); walk to /
  esac
  if ! has_js_deps; then
    echo "⏭️   [js] package.json has no dependencies; skipping install"
    return 0
  fi
  # Detect the package manager for THIS package. Correctness matters more
  # than speed here: pnpm and npm lay out node_modules completely
  # differently (symlink farm vs. flat tree), so running the wrong one
  # either errors out or silently produces a broken tree.
  #
  # Detection walks up from the package dir, nearest marker wins, with two
  # marker classes:
  #   - workspace markers: pnpm-workspace.yaml is authoritative for the
  #     whole tree below it; the packageManager field only counts in the
  #     package's own package.json (members of a pnpm workspace do NOT
  #     inherit it from the root — pnpm does not enforce it there, and
  #     inheriting it would mis-tag member packages that carry their own
  #     lockfile for a different PM)
  #   - lockfiles: authoritative for their own directory tree
  pm=""
  root_marker=""
  root_dir=""
  if command -v node >/dev/null 2>&1; then
    root_marker=$(node -e 'try{const m=require(process.argv[1]).packageManager||"";console.log(m.split("@")[0])}catch(e){}' "$pkg_dir/package.json" 2>/dev/null)
    [ -n "$root_marker" ] && root_dir="$pkg_dir"
  fi
  dir="$pkg_dir"
  while : ; do
    if [ -f "$dir/pnpm-workspace.yaml" ]; then root_marker="pnpm"; root_dir="$dir"; break; fi
    if [ "$dir" = "$walk_root" ] || [ "$dir" = "/" ]; then break; fi
    dir=$(dirname "$dir")
  done
  lock_dir=""
  dir="$pkg_dir"
  while : ; do
    if [ -f "$dir/pnpm-lock.yaml" ]; then pm="pnpm"; lock_dir="$dir"; break; fi
    if [ -f "$dir/yarn.lock" ]; then pm="yarn"; lock_dir="$dir"; break; fi
    if [ -f "$dir/bun.lockb" ] || [ -f "$dir/bun.lock" ]; then pm="bun"; lock_dir="$dir"; break; fi
    if [ -f "$dir/package-lock.json" ] || [ -f "$dir/npm-shrinkwrap.json" ]; then pm="npm"; lock_dir="$dir"; break; fi
    if [ "$dir" = "$walk_root" ] || [ "$dir" = "/" ]; then break; fi
    dir=$(dirname "$dir")
  done
  # Member of a pnpm workspace whose own lockfile points at a DIFFERENT
  # PM? That's contradictory input: running the other PM would build a
  # foreign node_modules layout inside the workspace. Refuse, loudly.
  if [ -n "$root_dir" ] && [ "$root_dir" != "$pkg_dir" ] && [ "$root_marker" = "pnpm" ] \
     && [ "$pm" != "pnpm" ] && [ -n "$pm" ]; then
    case "$lock_dir" in
      "$root_dir"/*|"$root_dir")
        echo "⚠️  [js] $pkg_dir is inside a pnpm workspace ($root_dir) but has its own lockfile in $lock_dir — skipping; remove the stray lockfile or move the package out of the workspace"
        return 0
        ;;
    esac
  fi
  # A non-pnpm workspace marker beats a cross-PM lockfile found below or
  # at the workspace root, but a lockfile strictly above the marker
  # belongs to an outer project that wraps this one — leave it alone.
  if [ -n "$root_dir" ] && [ "$root_dir" != "$pkg_dir" ] && [ "$root_marker" != "$pm" ]; then
    case "$lock_dir" in
      "$root_dir"/*|"$root_dir"|"") pm="$root_marker" ;;
    esac
  fi
  if [ -z "$pm" ]; then
    pm="$root_marker"
  fi
  # An inherited pnpm lockfile (from a parent dir, no pnpm-workspace.yaml)
  # does NOT make this package part of a workspace: installing at the
  # parent would leave this package's deps uninstalled. Treat it as a
  # plain no-lockfile package instead.
  if [ "$pm" = "pnpm" ] && [ -n "$lock_dir" ] && [ "$lock_dir" != "$pkg_dir" ] \
     && ! { [ -n "$root_dir" ] && [ -f "$root_dir/pnpm-workspace.yaml" ]; }; then
    pm=""
    lock_dir=""
  fi
  # Inside a pnpm workspace the PM is pnpm even for packages that declare
  # no dependencies (has_js_deps may have skipped them already; be safe) —
  # covered by the workspace install and reported via the sentinel.
  if [ -z "$pm" ] && [ -n "$root_dir" ] && [ -f "$root_dir/pnpm-workspace.yaml" ]; then
    pm="pnpm"
  fi

  case "$pm" in
  pnpm)
    if ! command -v pnpm >/dev/null 2>&1; then
      echo "⚠️  [js] pnpm project but pnpm not installed; skipping (NOT falling back to npm — it would produce a broken node_modules)"
      return 0
    fi
    # pnpm keeps a single global content-addressed store and hard-links
    # from it into node_modules. Every worktree's install against the
    # same store is mostly link-creation — dramatically faster than
    # npm ci's fresh extract of every tarball.
    # If this package sits inside a pnpm workspace (pnpm-workspace.yaml),
    # run the install at the workspace root: pnpm install covers all
    # member projects in one run (recursive-install defaults to true), and
    # reporting the root back lets the caller skip the members entirely.
    ws_dir=""
    if [ -n "$root_dir" ] && [ -f "$root_dir/pnpm-workspace.yaml" ]; then
      ws_dir="$root_dir"
    fi
    if [ -n "$ws_dir" ]; then
      if [ "$pkg_dir" = "$ws_dir" ]; then
        echo "📦  [js] pnpm install --frozen-lockfile (workspace root)"
      else
        echo "📦  [js] pnpm install --frozen-lockfile (workspace root $ws_dir; covers $pkg_dir)"
      fi
      (cd "$ws_dir" && pnpm install --frozen-lockfile) || {
        echo "⚠️  [js] pnpm-lock.yaml out of sync with package.json; falling back to pnpm install"
        (cd "$ws_dir" && pnpm install) || echo "⚠️  [js] pnpm install failed ($ws_dir)"
      }
      echo "WT_JS_WS_ROOT:$ws_dir" >&2
    elif [ -f "pnpm-lock.yaml" ]; then
      echo "📦  [js] pnpm install --frozen-lockfile (pnpm-lock.yaml found)"
      pnpm install --frozen-lockfile || {
        echo "⚠️  [js] pnpm-lock.yaml out of sync with package.json; falling back to pnpm install"
        pnpm install || echo "⚠️  [js] pnpm install failed"
      }
    else
      echo "📦  [js] pnpm install (no lockfile; will create pnpm-lock.yaml)"
      pnpm install || echo "⚠️  [js] pnpm install failed"
    fi
    ;;
  yarn)
    if ! command -v yarn >/dev/null 2>&1; then
      echo "⚠️  [js] yarn project but yarn not installed; skipping"
      return 0
    fi
    # Yarn 2+ (Berry) uses --immutable; Yarn 1 uses --frozen-lockfile.
    # .yarnrc.yml lives at the workspace root in a monorepo, so check
    # upward (capped at the worktree root), not just this directory.
    yarnrc=""
    dir="$pkg_dir"
    while : ; do
      [ -f "$dir/.yarnrc.yml" ] && yarnrc="$dir/.yarnrc.yml" && break
      if [ "$dir" = "$walk_root" ] || [ "$dir" = "/" ]; then break; fi
      dir=$(dirname "$dir")
    done
    if [ -n "$yarnrc" ]; then
      echo "📦  [js] yarn install --immutable (yarn.lock + $yarnrc found)"
      yarn install --immutable || {
        echo "⚠️  [js] yarn.lock out of sync with package.json; falling back to yarn install"
        yarn install || echo "⚠️  [js] yarn install failed"
      }
    else
      echo "📦  [js] yarn install --frozen-lockfile (yarn.lock found)"
      yarn install --frozen-lockfile || {
        echo "⚠️  [js] yarn.lock out of sync with package.json; falling back to yarn install"
        yarn install || echo "⚠️  [js] yarn install failed"
      }
    fi
    ;;
  bun)
    if ! command -v bun >/dev/null 2>&1; then
      echo "⚠️  [js] bun project but bun not installed; skipping"
      return 0
    fi
    echo "📦  [js] bun install --frozen-lockfile (bun lockfile found)"
    bun install --frozen-lockfile || {
      echo "⚠️  [js] bun.lockb out of sync with package.json; falling back to bun install"
      bun install || echo "⚠️  [js] bun install failed"
    }
    ;;
  npm)
    if ! command -v npm >/dev/null 2>&1; then
      echo "⚠️  [js] npm project but npm not installed; skipping"
      return 0
    fi
    # npm has no shared store; every worktree gets a full node_modules.
    # npm ci still wins over npm install here: it skips resolution and
    # extracts straight from the lockfile + user cache. --prefer-offline
    # leans on the cache harder without failing on misses.
    if [ -n "$lock_dir" ]; then
      # Run npm ci where the lockfile lives (usually this dir, but an
      # inherited lockfile from a parent dir must run there).
      echo "📦  [js] npm ci --prefer-offline --no-audit --no-fund ($lock_dir/package-lock.json)"
      (cd "$lock_dir" && npm ci --prefer-offline --no-audit --no-fund) || echo "⚠️  [js] npm ci failed ($lock_dir)"
    else
      echo "📦  [js] npm install (no lockfile)"
      npm install || echo "⚠️  [js] npm install failed"
    fi
    ;;
  *)
    # No lockfile anywhere and no packageManager field: unknown project.
    # Prefer pnpm if available (its global store makes it the fastest and
    # its lockfile is cheap to generate), else npm.
    if command -v pnpm >/dev/null 2>&1; then
      echo "📦  [js] pnpm install (no lockfile, defaulting to pnpm)"
      pnpm install || echo "⚠️  [js] pnpm install failed"
    elif command -v npm >/dev/null 2>&1; then
      echo "📦  [js] npm install (no lockfile)"
      npm install || echo "⚠️  [js] npm install failed"
    else
      echo "⚠️  [js] package.json found but no pnpm/yarn/bun/npm available"
    fi
    ;;
  esac
}

# Python environments by lockfile / manifest priority.
install_python() {
  (cd "$1" && _install_python_body)
}

_install_python_body() {
  # A lockfile for a tool that isn't installed must NOT silently degrade
  # to a pip venv: the pinned constraints would be ignored entirely and
  # the resulting environment would not match the lockfile. Warn + skip,
  # same policy as the JS branches.
  if [ -f "poetry.lock" ] && ! command -v poetry >/dev/null 2>&1; then
    echo "⚠️  [py] poetry.lock found but poetry not installed; skipping (NOT falling back to pip — it would ignore the lockfile)"
    return 0
  fi
  if [ -f "uv.lock" ] && ! command -v uv >/dev/null 2>&1; then
    echo "⚠️  [py] uv.lock found but uv not installed; skipping (NOT falling back to pip)"
    return 0
  fi
  if [ -f "Pipfile.lock" ] && ! command -v pipenv >/dev/null 2>&1; then
    echo "⚠️  [py] Pipfile.lock found but pipenv not installed; skipping (NOT falling back to pip)"
    return 0
  fi
  if [ -f "poetry.lock" ]; then
    # poetry install is strict when poetry.lock exists; it never updates it.
    echo "🐍  [py] poetry install (poetry.lock found)"
    poetry install || echo "⚠️  [py] poetry install failed"
  elif [ -f "uv.lock" ]; then
    # --locked: fail if uv.lock is out of sync with pyproject.toml.
    echo "🐍  [py] uv sync --locked (uv.lock found)"
    uv sync --locked || {
      echo "⚠️  [py] uv.lock out of sync with pyproject.toml; falling back to uv sync"
      uv sync || echo "⚠️  [py] uv sync failed"
    }
  elif [ -f "Pipfile.lock" ]; then
    # --deploy: fail if Pipfile.lock is out of date; never regenerates it.
    echo "🐍  [py] pipenv install --dev --deploy (Pipfile.lock found)"
    pipenv install --dev --deploy || {
      echo "⚠️  [py] Pipfile.lock out of sync with Pipfile; falling back to pipenv install --dev"
      pipenv install --dev || echo "⚠️  [py] pipenv install failed"
    }
  elif [ ! -f "requirements.txt" ]; then
    # Only a pyproject.toml (or bare Pipfile): don't create an empty venv
    # and hit the network for nothing — same "no deps, skip" policy as JS.
    echo "⏭️   [py] no requirements.txt; skipping venv setup"
    return 0
  else
    if ! command -v python3 >/dev/null 2>&1; then
      echo "⚠️  [py] python3 not available; skipping"
      return 0
    fi
    if [ ! -d ".venv" ]; then
      echo "🐍  [py] creating .venv"
      # python3-venv is a separate package on Debian/Ubuntu; its absence
      # must not abort the hook.
      python3 -m venv .venv || {
        echo "⚠️  [py] python3 -m venv failed (missing python3-venv?); skipping"
        return 0
      }
    fi
    echo "🐍  [py] installing requirements into .venv"
    .venv/bin/python -m pip install --upgrade pip || echo "⚠️  [py] pip upgrade failed (offline?); continuing"
    .venv/bin/pip install -r requirements.txt || echo "⚠️  [py] pip install -r requirements.txt failed"
  fi
}

# Recursively scan the worktree for project roots (node_modules excluded).
scan_projects() {
  search_root="$1"
  # Workspace roots already installed by a member (or the root itself).
  # Newline-separated (not space-separated) so paths containing spaces —
  # common on macOS — don't get split apart.
  js_ws_roots=""

  # Python first so requirements.txt dirs don't shadow more specific
  # tools. Dedup by DIRECTORY: a dir matching several markers
  # (pyproject.toml + uv.lock is the norm) must be installed only once.
  find "$search_root" -maxdepth 3 \( -name node_modules -o -name .git -o -name .venv \) -prune \
    -o \( -name "requirements.txt" -o \
    -name "pyproject.toml" -o \
    -name "Pipfile.lock" -o \
    -name "uv.lock" -o \
    -name "poetry.lock" \
    \) -print 2>/dev/null | while IFS= read -r marker; do dirname "$marker"; done | sort -u | while IFS= read -r dir; do
    install_python "$dir"
  done

  # JS/Node. Shallowest first: in a monorepo the workspace root must be
  # installed before its members, because pnpm install covers the whole
  # workspace in one run (recursive-install defaults to true). Members
  # whose workspace root was already installed are skipped — find's
  # traversal order is arbitrary, so we sort by depth instead of relying
  # on it. The depth prefix is tab-separated so paths with spaces survive.
  find "$search_root" -maxdepth 3 \( -name node_modules -o -name .git -o -name dist -o -name build -o -name .next -o -name coverage -o -name .venv \) -prune \
    -o -name "package.json" -print 2>/dev/null \
    | awk -F/ '{printf "%d\t%s\n", NF, $0}' | sort -n | cut -f2- \
    | while IFS= read -r marker; do
    dir=$(cd "$(dirname "$marker")" && pwd) || continue
    skip=""
    if [ -n "$js_ws_roots" ]; then
      while IFS= read -r ws; do
        case "$dir" in
          "$ws"|"$ws"/*) skip=1; echo "⏭️   [js] covered by workspace install at $ws; skipping ($dir)"; break ;;
        esac
      done <<EOF
$js_ws_roots
EOF
    fi
    [ -n "$skip" ] && continue
    # stderr carries both pnpm's own diagnostics and our sentinel line.
    # Route it through a temp file: harvest the sentinel, replay the rest.
    WT_HOOK_ERR_FILE=$(mktemp "${TMPDIR:-/tmp}/wt-hook.XXXXXX")
    install_js "$dir" 2>"$WT_HOOK_ERR_FILE"
    ws_root=$(sed -n 's/^WT_JS_WS_ROOT://p' "$WT_HOOK_ERR_FILE" | tail -1)
    awk '/^WT_JS_WS_ROOT:/{next} {print}' "$WT_HOOK_ERR_FILE" >&2
    rm -f "$WT_HOOK_ERR_FILE"
    WT_HOOK_ERR_FILE=""
    if [ -n "$ws_root" ]; then
      js_ws_roots="${js_ws_roots:+$js_ws_roots
}$ws_root"
    fi
  done

  # Go. `go mod download` never modifies go.mod/go.sum.
  if command -v go >/dev/null 2>&1; then
    find "$search_root" -maxdepth 3 \( -name node_modules -o -name .git \) -prune \
      -o -name "go.mod" -print 2>/dev/null | while IFS= read -r marker; do
      echo "🐹  [go] go mod download ($marker)"
      (cd "$(dirname "$marker")" && go mod download) || echo "⚠️  [go] go mod download failed ($(dirname "$marker"))"
    done
  fi

  # Rust. --locked asserts Cargo.lock is up to date; fall back if stale.
  if command -v cargo >/dev/null 2>&1; then
    find "$search_root" -maxdepth 3 \( -name target -o -name .git \) -prune \
      -o -name "Cargo.toml" -print 2>/dev/null | while IFS= read -r marker; do
      echo "🦀  [rust] cargo fetch --locked ($marker)"
      (cd "$(dirname "$marker")" && cargo fetch --locked) || {
        echo "⚠️  [rust] Cargo.lock out of sync with Cargo.toml; falling back to cargo fetch"
        (cd "$(dirname "$marker")" && cargo fetch) || echo "⚠️  [rust] cargo fetch failed ($(dirname "$marker"))"
      }
    done
  fi

  # Swift / iOS / macOS.
  #
  # SPM: resolve per Package.swift, skipping nested checkouts (.build) and
  # vendored sources (Sources). Package.resolved pins exact versions;
  # resolve never rewrites it unless requirements changed.
  if command -v swift >/dev/null 2>&1; then
    find "$search_root" -maxdepth 4 \( -name .build -o -name .git \) -prune \
      -o -name "Package.swift" -not -path "*/Sources/*" -print 2>/dev/null | while IFS= read -r marker; do
      dir=$(dirname "$marker")
      echo "🐦  [swift] swift package resolve ($dir)"
      (cd "$dir" && swift package resolve) || echo "⚠️  [swift] swift package resolve failed ($dir)"
    done
  fi

  # Ruby. Runs BEFORE CocoaPods: a Gemfile that pins the cocoapods gem is
  # the standard iOS setup, and `bundle exec pod` below needs that gem
  # installed first. Skip Gemfiles that declare no gems (installing those
  # would create a lockfile the project never had). Otherwise `bundle check`
  # succeeds only if the lockfile's gems are already installed, avoiding
  # a redundant install on repeat worktree creation.
  find "$search_root" -maxdepth 3 \( -name vendor -o -name .git \) -prune \
    -o -name "Gemfile" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if ! grep -Eq "^[[:space:]]*gem[[:space:]]" "$marker"; then
      echo "⏭️   [ruby] Gemfile declares no gems; skipping ($dir)"
      continue
    fi
    if command -v bundle >/dev/null 2>&1; then
      echo "💎  [ruby] bundle install ($dir)"
      (cd "$dir" && (bundle check || bundle install)) || echo "⚠️  [ruby] bundle install failed ($dir)"
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
  find "$search_root" -maxdepth 4 \( -name Pods -o -name .git \) -prune \
    -o -name "Podfile" -print 2>/dev/null | while IFS= read -r marker; do
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
      (cd "$dir" && $pod_cmd install) || echo "⚠️  [pods] pod install failed ($dir)"
    }
  done

  # Gradle (Android / JVM). Prefer the project-local wrapper (./gradlew),
  # which pins the Gradle version; use system gradle only as a fallback.
  # `gradle help` resolves the build's plugins + dependencies without
  # compiling. Skip subprojects (they have no settings.gradle and would
  # resolve the whole build again) — but a dir with its own wrapper is
  # always a root, and single-project builds may have no settings.gradle
  # at all.
  find "$search_root" -maxdepth 3 \( -name build -o -name .git -o -name node_modules \) -prune \
    -o \( -name "build.gradle" -o -name "build.gradle.kts" \) -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if [ ! -f "$dir/settings.gradle" ] && [ ! -f "$dir/settings.gradle.kts" ] && [ ! -f "$dir/gradlew" ]; then
      continue  # subproject; the root build covers it
    fi
    echo "🐘  [gradle] resolving dependencies ($dir)"
    if [ -x "$dir/gradlew" ]; then
      (cd "$dir" && ./gradlew help -q) || echo "⚠️  [gradle] dependency resolution failed ($dir)"
    elif command -v gradle >/dev/null 2>&1; then
      (cd "$dir" && gradle help -q) || echo "⚠️  [gradle] dependency resolution failed ($dir)"
    else
      echo "⚠️  [gradle] build.gradle found but no ./gradlew or gradle available"
    fi
  done

  # Flutter / Dart. Flutter projects have pubspec.yaml + pubspec.lock.
  find "$search_root" -maxdepth 3 \( -name .dart_tool -o -name .git -o -name build \) -prune \
    -o -name "pubspec.yaml" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if [ -f "$dir/pubspec.lock" ] && command -v flutter >/dev/null 2>&1; then
      echo "🎯  [flutter] flutter pub get ($dir)"
      (cd "$dir" && flutter pub get) || echo "⚠️  [flutter] flutter pub get failed ($dir)"
    elif command -v dart >/dev/null 2>&1; then
      echo "🎯  [dart] dart pub get ($dir)"
      (cd "$dir" && dart pub get) || echo "⚠️  [dart] dart pub get failed ($dir)"
    else
      echo "⚠️  [dart] pubspec.yaml found but no flutter/dart available"
    fi
  done

  # PHP. `composer install` is strict when composer.lock exists.
  find "$search_root" -maxdepth 3 \( -name vendor -o -name .git \) -prune \
    -o -name "composer.json" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if command -v composer >/dev/null 2>&1; then
      echo "🐘  [php] composer install ($dir)"
      (cd "$dir" && composer install --no-interaction --prefer-dist) || echo "⚠️  [php] composer install failed ($dir)"
    else
      echo "⚠️  [php] composer.json found but composer not available"
    fi
  done

  # .NET. Restore at the solution level when one exists (covers all
  # projects), otherwise per-project. --locked-mode asserts the lock file
  # but fails outright when the project has no packages.lock.json, so only
  # pass it when one exists. Skip .csproj/.fsproj in dirs that have a
  # .sln — the solution restore already covers them.
  find "$search_root" -maxdepth 3 \( -name bin -o -name obj -o -name .git \) -prune \
    -o \( -name "*.sln" -o -name "*.csproj" -o -name "*.fsproj" \) -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    case "$marker" in
      *.csproj|*.fsproj)
        if ls "$dir"/*.sln >/dev/null 2>&1; then
          continue  # covered by the solution restore
        fi
        ;;
    esac
    if command -v dotnet >/dev/null 2>&1; then
      if ls "$dir"/packages.lock.json "$dir"/*/packages.lock.json >/dev/null 2>&1; then
        echo "🔷  [.net] dotnet restore --locked-mode ($marker)"
        (cd "$dir" && dotnet restore "$marker" --locked-mode) || echo "⚠️  [.net] dotnet restore failed ($marker)"
      else
        echo "🔷  [.net] dotnet restore ($marker)"
        (cd "$dir" && dotnet restore "$marker") || echo "⚠️  [.net] dotnet restore failed ($marker)"
      fi
    else
      echo "⚠️  [.net] $marker found but dotnet not available"
    fi
  done

  # Java / Maven.
  find "$search_root" -maxdepth 3 \( -name target -o -name .git \) -prune \
    -o -name "pom.xml" -print 2>/dev/null | while IFS= read -r marker; do
    dir=$(dirname "$marker")
    if command -v mvn >/dev/null 2>&1; then
      echo "☕  [maven] dependency:go-offline ($dir)"
      (cd "$dir" && mvn -q dependency:go-offline -B) || echo "⚠️  [maven] dependency:go-offline failed ($dir)"
    else
      echo "⚠️  [maven] pom.xml found but mvn not available"
    fi
  done
}

scan_projects "$ROOT"

echo "✅  wt post-create hook finished"
