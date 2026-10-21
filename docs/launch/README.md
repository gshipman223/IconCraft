# IconCraft launch kit — v0.13.3

Drafts only. **Nothing in this folder has been posted anywhere.** Every file here is
ready-to-paste copy that still needs Gavin's eyes, the posting notes at the top of
each file checked, and a deliberate click from Gavin to go live.

IconCraft in one line: a free, MIT-licensed Mac app (signed and notarised DMG,
macOS 12.3+) that gives replacement and vibe-coded apps the original app's name and
icon — or your own — system-wide, leaves the app underneath unchanged, and makes
every change reversible. No telemetry.

- Website: https://iconcraft.co
- Repo: https://github.com/gshipman223/IconCraft
- v0.13.3 DMG: https://github.com/gshipman223/IconCraft/releases/latest/download/IconCraft-Mac-0.13.3.dmg
  (sha256 `21f8f4e3a2d66c6e9507bcb7ac4253f0fd41d8ee61e6a861013752635ce4444a`)

## What's here, where it goes, and in what order

Post in this order, one at a time, with space between them. Don't fire them all off
on the same day: each audience reacts differently, feedback from the first round
should be able to change the copy (or the app) before the next round, and turning up
in four places at once with the same pitch reads as a campaign rather than a person
showing something they made.

| Order | File | Where it goes | Why this slot |
| --- | --- | --- | --- |
| 1 | `storytold-discussion.md` | storytold/artcraft GitHub Discussions (Show and Tell style) | The ArtCraft community is the audience IconCraft was built for — PhotoCraft → Photoshop is the clearest example of what it does. Smallest, most relevant room first; also the right place to learn whether the suggested name/icon pairings are actually right. Post as honest community participation, unaffiliated, and accept removal gracefully if the maintainers prefer it gone. |
| 2 | `reddit-macapps.md` | r/macapps | Friendly to independent Mac apps when you disclose you're the developer and stick around for questions. A wider Mac audience than ArtCraft's, and good at surfacing rough edges before bigger venues. Check the subreddit's self-promotion rules on the day. |
| 3 | `show-hn.md` | Hacker News — Show HN | Biggest and bluntest audience. By now the obvious questions (reversibility, app updates, what happens under the hood, written in Go) will have come up once already, so the answers are ready. Submit once, don't re-post if it doesn't land, never ask for upvotes. |
| 4 | `product-hunt.md` | Product Hunt | Most "launch-shaped" venue, so it benefits most from a site, screenshots and copy already tested by the earlier rounds. Needs a free day to reply to comments, so schedule it deliberately rather than squeezing it in. |

Not a post. These keep working once they are in, with no account of yours in the loop:

- Working install today: `brew install --cask gshipman223/iconcraft/iconcraft`
  from [gshipman223/homebrew-iconcraft](https://github.com/gshipman223/homebrew-iconcraft).
  `brew audit` passes on this tap.
- Official cask: [homebrew-cask PR #295884](https://github.com/Homebrew/homebrew-cask/pull/295884).
  Fetch, install and the checksum pass. The audit still fails for two policy
  reasons that are not a cask bug: `iconcraft.co` was registered on 2026-10-20
  (Homebrew wants 30 days) and a self-submitted repo needs 225 stars, 90 forks
  and 90 watchers. Leave the PR open.
- [awesome-mac PR #3713](https://github.com/jaywcjlove/awesome-mac/pull/3713)
  is open and waiting on that list's maintainer. The entry links to iconcraft.co.

## Shared rules for all of it

- We do not own, and are not affiliated with, ArtCraft / Storytold — or Adobe,
  Microsoft, Figma or anyone else whose app names appear as examples. Every draft
  says so, out loud. Keep it that way in replies too.
- Honest community participation, never spam: one post per venue, no DMs, no
  dropping the link into unrelated threads/issues, no vote soliciting, anywhere.
- No hype and no false claims. Only say what's true today: free, MIT, signed and
  notarised DMG, macOS 12.3+, restores the original name/icon system-wide, app
  underneath unchanged, every change reversible, no telemetry.
- Be present after posting. Answer criticism straight, log real bugs as GitHub
  issues, and thank people for corrections — the equivalents suggestions in
  particular are "verify before applying", and community corrections improve them.

## Tracking notes

Measure quietly, before and after each post — one at a time is what makes the
numbers mean anything.

- Baseline at v0.13.3 launch: **52 total release downloads**. Record the per-release
  download count on GitHub before each post goes out, then again a few days after,
  so growth can be attributed to the right venue rather than guessed at later.
- GitHub Traffic (repo → Insights → Traffic) only keeps about two weeks of history:
  check **referrers** after each post and write down what came from
  `github.com/storytold/artcraft`, `reddit.com`, `news.ycombinator.com` and
  `producthunt.com` while the data still exists. Note clones/views too, same reason.
- Suggested log, fill in as you go:

  | Date | Venue | Post URL | Downloads before | Downloads after (few days) | Referrer seen in Traffic? | Notes / issues raised |
  | --- | --- | --- | --- | --- | --- | --- |
  |  | ArtCraft Discussions (storytold/artcraft) |  |  |  |  |  |
  |  | r/macapps |  |  |  |  |  |
  |  | Show HN |  |  |  |  |  |
  |  | Product Hunt |  |  |  |  |  |

- IconCraft has no telemetry, by design — so GitHub Traffic referrers, release
  download counts, new issues/stars and what people actually say in replies are the
  whole picture. That's fine. Don't add analytics to the app or site just to feed
  this table without thinking it through first.
