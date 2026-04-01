# Maintainer: Nick <nick@example.com>
pkgname=profuse-git
pkgver=r1.0000000
pkgrel=1
pkgdesc="Proton Drive FUSE client for Linux — mounts your Proton Drive as a local filesystem"
arch=('x86_64' 'aarch64')
url="https://github.com/nick/profuse"
license=('GPL-3.0-or-later')
depends=(
    'fuse3'          # FUSE kernel interface
    'libsecret'      # Secret Service implementation (GNOME Keyring / KWallet)
)
makedepends=(
    'go'
    'git'
)
optdepends=(
    'gnome-keyring: Secret Service backend for GNOME desktops'
    'kwallet: Secret Service backend for KDE desktops'
)
provides=('profuse')
conflicts=('profuse')
source=("git+${url}.git")
sha256sums=('SKIP')

pkgver() {
    cd "$srcdir/profuse"
    printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)"
}

build() {
    cd "$srcdir/profuse"

    export CGO_ENABLED=0
    export GOPATH="$srcdir/gopath"
    export GOMODCACHE="$GOPATH/pkg/mod"

    go build \
        -trimpath \
        -mod=readonly \
        -modcacherw \
        -ldflags "-s -w -X main.version=${pkgver}" \
        -o profuse \
        ./cmd/
}

package() {
    cd "$srcdir/profuse"

    # Binary
    install -Dm755 profuse \
        "$pkgdir/usr/bin/profuse"

    # systemd user service
    install -Dm644 contrib/systemd/profuse.service \
        "$pkgdir/usr/lib/systemd/user/profuse.service"
}
