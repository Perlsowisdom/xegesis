# Partner Center checklist (what you do yourself)

The agent and CI cannot do these: they need your identity and your agreement to Microsoft's terms.
Do them once, in this order. The first submission can take several days to certify.

## One time

1. **Create the developer account.** Go to https://partner.microsoft.com/dashboard/registration and
   choose **Individual** (free for individuals). Sign in with a Microsoft account, then verify your
   identity as asked. No payment is needed for an individual account.
2. **Reserve the name.** Apps and games → New product → MSIX or PWA app → reserve **Xegesis**. If it is
   taken, pick a variant such as "Xegesis Bible Study"; the name you reserve is what the Store shows.
3. **Copy the three identity values** from Product management → Product identity:
   - Package/Identity/Name
   - Package/Identity/Publisher (starts with `CN=`)
   - Package/Identity/PublisherDisplayName
4. **Give them to the build.** In the GitHub repo, Settings → Secrets and variables → Actions →
   **Variables** tab, add `XEGESIS_MSIX_IDENTITY`, `XEGESIS_MSIX_PUBLISHER` and
   `XEGESIS_MSIX_PUBLISHER_DISPLAY` with those values. (Variables, not secrets: they are not secret.)
   Without them, the package carries `PLACEHOLDER` values and Partner Center will refuse it.

## The listing (Product → Store listing)

5. Paste the text from `docs/store/listing.md` (short description, description, search terms) and the
   features from `docs/store/features.md`.
6. Upload the screenshots from `docs/store/screenshots/` (1366x768, at least one; up to ten) and the
   logos `docs/store/logo-300.png` (the 300x300 Store logo) and `logo-1080.png` (the 1080x1080 one).
7. Privacy policy link: `https://github.com/Perlsowisdom/xegesis/blob/main/docs/store/privacy-policy.md`
   (the repository must be public by then).

## Properties and age rating

8. Category: Books & reference. Support contact: your email or the GitHub issues page.
9. **Age rating**: Product → Age ratings → answer the questionnaire (see `listing.md`). Expect "Everyone".
10. Pricing: Free. Availability: the markets you want. No in-app purchases.

## Submit

11. Download the `.msix` from the draft GitHub release (made by the release workflow).
    **Upload it unsigned**: the Store signs it at certification.
12. Packages → upload the `.msix`. Under Notes for certification, say: "Desktop app (runFullTrust). Works
    offline. Network features are off until the user turns them on; see the privacy policy."
13. Submit for certification. Fix anything the certification report lists, then repeat from step 11 with a
    new version.

## Each later release

Follow `docs/releasing.md`: publish the GitHub release, then upload the new `.msix` in a new submission
(Product → Update). The Store version must be higher than the last one submitted.
