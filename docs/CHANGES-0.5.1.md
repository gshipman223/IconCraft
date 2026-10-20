# 0.5.1 — exhaustive original-icon search

Fixes "Not found · choose icon" appearing when a genuine icon does exist
(AutoCAD was the reported case: autodesk.com returns HTTP 403 to programmatic
fetches, and AutoCAD is not sold as a Mac App Store app).

The resolver now tries, in parallel, before it may say "Not found":
1. Publisher product pages (US + regional + help/download pages), including
   icon URLs embedded in page JSON/script data on JS-heavy sites.
2. Fixed publisher CDN icon paths (Adobe etc.).
3. Verified App Store listing by exact track ID (US + AU).
4. Verified App Store **search**: seller must be the publisher, track name
   must equal the product, and the bundle ID must be known/variant-suffixed —
   knockoffs (e.g. "CAD Plus") are rejected. AutoCAD resolves this way via
   Autodesk's own listing (track 393149734, artwork from mzstatic).
5. Saved original from a previous successful check (offline).
6. The genuine installed product's own icon — exact bundle ID, year-suffixed
   bundle IDs (AutoCAD2025), or exact product name.

If all layers fail, the error now names every layer and what it returned
("publisher page www.autodesk.com: source returned HTTP 403; App Store
search: …; also checked: … installed apps (no genuine AutoCAD installed)"),
so "Not found" means it was actually looked for everywhere above.
Verified live on the test Mac: CADCraft → genuine AutoCAD icon (see
docs/BUILD-VALIDATION.json). Avid (Pro Tools) is likewise 403 + not on the
store, so it resolves via layer 6 when Pro Tools is installed, else reports
the full checklist honestly.
