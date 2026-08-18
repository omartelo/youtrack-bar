# Maintainer: omartelo <meopedevts@proton.me>
pkgname=youtrack-tui-bin
pkgver=@VERSION@
pkgrel=1
pkgdesc="Read-only terminal UI for browsing YouTrack issues"
arch=('x86_64' 'aarch64')
url="https://github.com/omartelo/youtrack-tui"
license=('MIT')
provides=('youtrack-tui')
conflicts=('youtrack-tui')
source_x86_64=("${pkgname}-${pkgver}-x86_64.tar.gz::${url}/releases/download/v${pkgver}/youtrack-tui-${pkgver}-linux-amd64.tar.gz")
source_aarch64=("${pkgname}-${pkgver}-aarch64.tar.gz::${url}/releases/download/v${pkgver}/youtrack-tui-${pkgver}-linux-arm64.tar.gz")
sha256sums_x86_64=('@SHA_AMD64@')
sha256sums_aarch64=('@SHA_ARM64@')

package() {
  install -Dm755 youtrack-tui "${pkgdir}/usr/bin/youtrack-tui"
  install -Dm644 LICENSE "${pkgdir}/usr/share/licenses/${pkgname}/LICENSE"
}
