# Maintainer: Nick <nick@example.com>
pkgname=protondriveclient-git
pkgver=r1.0000000
pkgrel=1
pkgdesc="Proton Drive FUSE client for Linux — mounts your Proton Drive as a local filesystem"
arch=('x86_64' 'aarch64')
url="https://github.com/nick/protondriveclient"
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
provides=('protondriveclient')
conflicts=('protondriveclient')
source=("git+${url}.git")
sha256sums=('SKIP')

pkgver() {
    cd "$srcdir/protondriveclient"
    printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)"
}

build() {
    cd "$srcdir/protondriveclient"

    export CGO_ENABLED=0
    export GOPATH="$srcdir/gopath"
    export GOMODCACHE="$GOPATH/pkg/mod"

    go build \
        -trimpath \
        -mod=readonly \
        -modcacherw \
        -ldflags "-s -w -X main.version=${pkgver}" \
        -o protondrive \
        ./cmd/
}

package() {
    cd "$srcdir/protondriveclient"

    # Binary
    install -Dm755 protondrive \
        "$pkgdir/usr/bin/protondrive"

    # systemd user service
    install -Dm644 contrib/systemd/protondrive.service \
        "$pkgdir/usr/lib/systemd/user/protondrive.service"
}
