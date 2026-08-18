# Rendered by .github/workflows/release.yml and published to
# omartelo/homebrew-tap. Edit this template, never the generated formula.
class YoutrackTui < Formula
  desc "Read-only terminal UI for browsing YouTrack issues"
  homepage "https://github.com/omartelo/youtrack-tui"
  license "MIT"
  version "@VERSION@"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/omartelo/youtrack-tui/releases/download/v#{version}/youtrack-tui-#{version}-darwin-arm64.tar.gz"
      sha256 "@SHA_DARWIN_ARM64@"
    else
      url "https://github.com/omartelo/youtrack-tui/releases/download/v#{version}/youtrack-tui-#{version}-darwin-amd64.tar.gz"
      sha256 "@SHA_DARWIN_AMD64@"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/omartelo/youtrack-tui/releases/download/v#{version}/youtrack-tui-#{version}-linux-arm64.tar.gz"
      sha256 "@SHA_LINUX_ARM64@"
    else
      url "https://github.com/omartelo/youtrack-tui/releases/download/v#{version}/youtrack-tui-#{version}-linux-amd64.tar.gz"
      sha256 "@SHA_LINUX_AMD64@"
    end
  end

  def install
    bin.install "youtrack-tui"
  end

  test do
    assert_match "path to config.yml", shell_output("#{bin}/youtrack-tui -h 2>&1")
  end
end
