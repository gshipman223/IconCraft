# Reference copy of the cask. The installable one lives in
# github.com/gshipman223/homebrew-iconcraft (works today). The same cask is
# proposed for the main tap in Homebrew/homebrew-cask PR #295884, which cannot
# merge until the homepage is 30 days old and the repo meets Homebrew's
# notability bar. Keep the versioned release URL, not /releases/latest.

cask "iconcraft" do
  version "0.13.3"
  sha256 "21f8f4e3a2d66c6e9507bcb7ac4253f0fd41d8ee61e6a861013752635ce4444a"

  url "https://github.com/gshipman223/IconCraft/releases/download/v#{version}/IconCraft-Mac-#{version}.dmg"
  name "IconCraft"
  desc "Give any Mac app the name and icon you already know"
  homepage "https://iconcraft.co"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on macos: :monterey

  app "IconCraft.app"

  zap trash: [
    "~/Library/Application Support/IconCraft",
    "~/Library/LaunchAgents/io.iconcraft.keepapplied.plist",
    "~/Library/LaunchAgents/io.iconcraft.login.plist",
  ]
end
