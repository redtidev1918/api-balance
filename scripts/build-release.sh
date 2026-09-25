#!/usr/bin/env bash
# ============================================================
#  api-balance release build script
#  Builds linux/amd64 and linux/arm64 standalone binaries,
#  .deb packages, and a checksums.txt for a GitHub Release.
#
#  Usage:  ./scripts/build-release.sh 0.1.0
#  Output: dist/
# ============================================================
set -euo pipefail

VERSION="${1:?usage: build-release.sh <version> (e.g. 0.1.0)}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DIST="$ROOT/dist"
DEB_ROOT="$DIST/deb-root"
rm -rf "$DIST"
mkdir -p "$DIST"

# --- helper to build for one arch ---
build_arch() {
  local arch="$1"
  local goarch="$2"
  echo ">> building linux/${goarch}..."
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build \
    -trimpath -ldflags "-s -w -X github.com/redtidev1918/api-balance/internal/cli.Version=${VERSION}" \
    -o "$DIST/api-balance-linux-${goarch}" ./cmd/api-balance
  echo "   done: api-balance-linux-${goarch}"
}

# --- package .deb for one arch ---
build_deb() {
  local goarch="$1"
  local arch="$2"   # deb arch: amd64 / arm64
  local pkg="api-balance_${VERSION}_${arch}"
  local src="$DIST/api-balance-linux-${goarch}"
  local root="$DEB_ROOT/${pkg}"

  echo ">> packaging ${pkg}..."
  mkdir -p "$root/usr/bin"
  mkdir -p "$root/lib/systemd/system"
  mkdir -p "$root/usr/share/doc/api-balance"
  mkdir -p "$root/usr/share/man/man1"
  mkdir -p "$root/usr/lib/api-balance"

  install -m 0755 "$src" "$root/usr/bin/api-balance"
  install -m 0644 packaging/systemd/api-balance.service "$root/lib/systemd/system/api-balance.service"
  install -m 0644 docs/config.example.yaml "$root/usr/share/doc/api-balance/config.example.yaml"
  gzip -9 -c docs/api-balance.1 > "$root/usr/share/man/man1/api-balance.1.gz"
  install -m 0644 docs/api-balance.1 "$root/usr/lib/api-balance/api-balance.1"

  # control file
  mkdir -p "$root/DEBIAN"
  cat > "$root/DEBIAN/control" <<EOF
Package: api-balance
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${arch}
Maintainer: redtidev1918 <redtidev1918@users.noreply.github.com>
Depends: libc6 (>= 2.17)
Description: multi-provider AI API balance/quota CLI monitor
 Lightweight CLI to query balances and quotas across AI API providers
 (DeepSeek, OpenRouter, SiliconFlow, Moonshot, MiniMax, custom), run a
 watch loop with Telegram/webhook alerts and alert deduplication.
EOF

  cat > "$root/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
# Do NOT auto-start the service. User opts in explicitly:
#   sudo systemctl enable --now api-balance
mkdir -p /var/lib/api-balance
if ! getent group api-balance >/dev/null 2>&1; then
  addgroup --system api-balance >/dev/null 2>&1 || groupadd --system api-balance
fi
if ! getent passwd api-balance >/dev/null 2>&1; then
  adduser --system --ingroup api-balance --no-create-home --shell /usr/sbin/nologin api-balance >/dev/null 2>&1 \
    || useradd --system --gid api-balance --no-create-home --shell /usr/sbin/nologin api-balance
fi
chown api-balance:api-balance /var/lib/api-balance 2>/dev/null || true
exit 0
EOF
  chmod 0755 "$root/DEBIAN/postinst"

  # keep user config on upgrade/removal; only drop on purge
  cat > "$root/DEBIAN/prerm" <<'EOF'
#!/bin/sh
# stop the service if running when removed (keep config unless purged)
if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
  if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    systemctl stop api-balance.service 2>/dev/null || true
  fi
fi
exit 0
EOF
  chmod 0755 "$root/DEBIAN/prerm"

  cat > "$root/DEBIAN/postrm" <<'EOF'
#!/bin/sh
# config directories are kept unless purged
case "$1" in
  purge)
    rm -rf /var/lib/api-balance
    ;;
  remove|upgrade)
    :
    ;;
esac
exit 0
EOF
  chmod 0755 "$root/DEBIAN/postrm"

  # conffiles: config example is a doc; system config is not shipped to avoid
  # clobbering user config. We keep the example under /usr/share/doc.

  fakeroot dpkg-deb --build "$root" "$DIST/${pkg}.deb" >/dev/null 2>&1 \
    || (cd "$root" && dpkg-deb --build . "$ROOT/dist/${pkg}.deb" >/dev/null)
  echo "   done: ${pkg}.deb"
}

# --- main ---
build_arch amd64 amd64
build_arch arm64 arm64
build_deb amd64 amd64
build_deb arm64 arm64

# --- checksums ---
echo ">> writing checksums.txt..."
(cd "$DIST" && sha256sum api-balance-linux-amd64 api-balance-linux-arm64 api-balance_${VERSION}_amd64.deb api-balance_${VERSION}_arm64.deb > checksums.txt)

echo
echo "=============================================="
echo "  Release artifacts in $DIST:"
ls -1 "$DIST"
echo "=============================================="