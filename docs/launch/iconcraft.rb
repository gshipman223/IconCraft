# DRAFT ONLY — Homebrew cask for IconCraft.
#
# This file is a prepared draft. It has NOT been submitted to Homebrew, and
# should not be submitted anywhere until Gavin decides to do so deliberately
# (homebrew-cask has contribution rules about notability, signing/notarisation
# and cask naming that need a proper check at submission time).
#
# If/when submitting: this would normally live in Homebrew/homebrew-cask as
# Casks/i/iconcraft.rb, pass `brew audit --cask iconcraft` and
# `brew style iconcraft` locally first, and be opened as a PR there.
# Note the url below is the "latest" release URL given for v0.13.3; for a real
# cask submission, prefer the versioned release URL pattern
# (https://github.com/gshipman223/IconCraft/releases/download/v#{version}/...)
# if that asset URL exists, so old versions stay fetchable.

cask "iconcraft" do
  version "0.13.3"
  sha256 "21f8f4e3a2d66c6e9507bcb7ac4253f0fd41d8ee61e6a861013752635ce4444a"

  url "https://github.com/gshipman223/IconCraft/releases/latest/download/IconCraft-Mac-0.13.3.dmg"
  name "IconCraft"
  desc "Restore original app names and icons, or customise any macOS app"
  homepage "https://iconcraft.co"

  depends_on macos: ">= :monterey"

  app "IconCraft.app"

  zap trash: "~/Library/Application Support/IconCraft"
end
