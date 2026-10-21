# DRAFT — storytold/artcraft GitHub Discussions (Show and Tell)

> Where: https://github.com/storytold/artcraft/discussions — pick the category that
> fits community projects / Show and Tell if there is one. If there isn't a suitable
> category, don't post it elsewhere in the repo. Issues are for bugs in ArtCraft, not this.
> Before posting: check the repo's CONTRIBUTING / discussion guidelines, attach the
> screenshot noted below, and be around to answer replies for a day or two.
> Do not post this anywhere else under the ArtCraft name, and do not DM people about it.

**Suggested title:** I made a little companion tool for ArtCraft apps on macOS — it gives them the original app's name and icon back

---

Hi all,

I made a little companion tool for ArtCraft apps on macOS, and I thought some people here might find it useful.

I use apps like PhotoCraft, and I kept losing them in my Dock and Launchpad because I was looking for the name and icon of the app it works like. So I built IconCraft: you point it at an app, pick the original name and icon (or your own), and that's how the app shows up across macOS — in the Dock, Finder and Launchpad. The app underneath doesn't change. It opens, runs and updates as before, it's just wearing a different name and icon on the outside. Every change can be undone, and it puts the original name and icon back exactly as they were.

It already knows the ArtCraft apps it finds on your Mac (it looks for the `ai.storyteller.*` apps), and it suggests the obvious counterparts — PhotoCraft → Adobe Photoshop, FilmCraft → Adobe Premiere Pro, FigCraft → Figma, GridCraft → Microsoft Excel, and so on. You always confirm the suggestion before anything changes, and you can pick a custom icon instead if you'd rather something else.

Here's what it looks like with a few ArtCraft apps listed, PhotoCraft → Photoshop included:

![IconCraft listing ArtCraft apps next to the original name and icon each one can use](https://iconcraft.co/assets/shot-apps.jpg)
<!-- Posting note: if that link doesn't render in Discussions, upload website/assets/shot-apps.jpg from the IconCraft repo as an attachment instead. -->

A few plain facts:

- Free and open source (MIT): https://github.com/gshipman223/IconCraft
- Website and download: https://iconcraft.co
- The Mac download is a signed and notarised DMG, so it opens normally — no right-click workaround. Needs macOS 12.3 or later.
- No account, no telemetry. It doesn't phone home to me.
- It isn't only for ArtCraft apps. It works on other Mac apps too, including apps you've built or generated yourself.

To be clear: I'm not affiliated with ArtCraft / Storytold, Adobe, Figma, Microsoft or any of the other app makers mentioned here, and neither is IconCraft. It's an independent, community-made tool. It doesn't include or redistribute anyone else's app — it just changes how an app already on your Mac is labelled and shown. If the ArtCraft maintainers would rather this post wasn't here, I'm happy for it to be removed, no hard feelings.

If you try it, I'd genuinely like to hear how it went — especially if an ArtCraft app wasn't detected, or a suggested name/icon pairing was wrong. Replies here are fine, or open an issue on the IconCraft repo and mention which ArtCraft app you were using.

Thanks for reading,
Gavin
