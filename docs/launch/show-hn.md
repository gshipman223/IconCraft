# DRAFT — Hacker News (Show HN)

> Where: https://news.ycombinator.com/submit
> HN rules for Show HN: it has to be something people can try, the title must start
> with "Show HN:", and don't ask for upvotes anywhere (including in the post, on
> other sites, or in person). Link to something usable — the site or the repo, not
> a signup page. Expect blunt technical questions and answer them straight.

**Title:**

Show HN: IconCraft – give Mac apps the name and icon you actually look for (free, MIT)

**URL:** https://iconcraft.co

<!-- If submitting as a text post instead, put the URL in the body below and link the
     repo as well. Don't submit both. -->

**Body / first comment text:**

Hi HN, I'm Gavin, I built IconCraft.

It's a small macOS app that renames and re-icons other apps on your Mac, system-wide — Dock, Finder, Launchpad. The app underneath is unchanged; only how it's presented changes, and every change is reversible back to the original name and icon.

The case that made me build it: I use third-party replacement apps — the ArtCraft suite is the obvious one (PhotoCraft, FilmCraft and friends) — and vibe-coded apps I've generated myself. They work like apps I already know, but they sit in the Dock under an unfamiliar name and icon, so I kept hunting for them under the name my brain expects. IconCraft detects the ArtCraft apps automatically (bundle IDs under `ai.storyteller.*`), suggests the counterpart — PhotoCraft → Photoshop, GridCraft → Excel, that sort of thing — and applies the name and icon once you confirm. Custom names and icons work for any app too.

Facts, up front:

- Free, MIT. Source: https://github.com/gshipman223/IconCraft
- It's written in Go, with no account and no telemetry.
- The release is a Developer ID signed and notarised DMG (v0.13.3), macOS 12.3+, Apple Silicon and Intel.
- To be explicit about what it doesn't do: it doesn't modify or redistribute the target app's code, and I'm not affiliated with ArtCraft/Storytold, Adobe, Microsoft or Figma. The names/icons I mentioned are the presentation you choose on your own machine; you confirm every suggestion before it's applied.

I'd value criticism on the approach as much as bug reports. If you think changing an app's presented identity is a footgun, I'm interested in where you'd draw the line — the design constraint I set myself was "app underneath untouched, everything reversible, nothing silent." Issues and discussion on GitHub are welcome.
