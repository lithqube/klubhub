# KlubHub website and GitHub Pages

The public umbrella site lives at **https://klubhub.io/**. KlubHub DJ is the first product; KlubHub Promoter and KlubHub Label are coming soon. Most DJ features will be open source, with some future SaaS offerings. No SaaS availability, pricing, or feature boundary is promised by this page.

## Structure and local checks

- `apps/site/public/`: semantic HTML, responsive CSS, favicon, sitemap, robots, and domain declaration.
- `apps/site/build.mjs`: dependency-free static build into `dist/site/`. Extracts the DJ application's `@theme` tokens and font declarations directly from `apps/dj/app/assets/css/styles.css`, and serves local OFL-licensed Latin font subsets from `apps/site/public/fonts/`. These were retrieved from Google Fonts (Space Grotesk, Inter, Manrope); their upstream OFL licenses are included alongside them. The app's existing font binaries fail browser decoding, so the site uses independent valid copies without modifying the application. No duplicated palette or third-party font requests.
- `apps/site/site.test.mjs`: Node tests for domain metadata, local links, design tokens, fonts, and product availability copy.
- `.github/workflows/pages.yml`: builds/tests pull requests and deploys main through the official Pages actions. Only `dist/site` is uploaded, never the application backend or repository root.

From the repository root:

```sh
pnpm exec nx run site:test
pnpm exec nx run site:build
python3 -m http.server 8088 --directory dist/site --bind 127.0.0.1
```

Visit `http://127.0.0.1:8088`. The page uses no JavaScript, cookies, analytics, forms, or backend. Fonts are served locally. Relative asset URLs also work under GitHub's repository subpath before the custom domain is enabled.

The CI intentionally avoids installing the Nuxt/Go workspace; its equivalent standalone commands are `node --test apps/site/site.test.mjs` and `node apps/site/build.mjs`.

## One-time publishing setup (repository/domain owner)

These are manual steps; adding files to the repository does not change GitHub settings or DNS.

1. Verify ownership of `klubhub.io` in the GitHub account/organization's **Settings → Pages**, using the TXT record GitHub supplies. Keep that record. Confirm Pages is available for the repository's visibility and GitHub plan.
2. In `lithqube/klubhub` → **Settings → Pages**, select **GitHub Actions** as the build source, not branch publishing.
3. Set the custom domain to **klubhub.io** and save **before changing DNS**, to reduce takeover risk. The included `CNAME` documents the intended host, but GitHub ignores it for custom Actions publishing: the Pages setting is required.
4. At your DNS provider, point the apex (`@`) to these GitHub Pages IPv4 addresses, replacing conflicting apex website A/AAAA/ALIAS records only. Preserve mail and verification records:

   | Type | Name | Value |
   | --- | --- | --- |
   | A | @ | 185.199.108.153 |
   | A | @ | 185.199.109.153 |
   | A | @ | 185.199.110.153 |
   | A | @ | 185.199.111.153 |
   | CNAME | www | lithqube.github.io |

   The `www` record is optional but recommended for GitHub's redirect to the apex. Do not point it to the repository path. Do not add wildcard DNS. If stale AAAA records exist, remove them or replace them with the current IPv6 values from the official guide below.
5. Merge/push the approved site changes to `main`, or run **Deploy KlubHub site** manually from Actions. The `github-pages` environment must allow deployment from `main`; approve it if protection rules require that.
6. Wait for GitHub's DNS check and TLS certificate issuance, then enable **Enforce HTTPS**. DNS propagation may take up to 24 hours.
7. Verify `https://klubhub.io/`, font/style requests, mobile layout, source/docs links, `https://klubhub.io/robots.txt`, and `https://klubhub.io/sitemap.xml`. If configured, confirm `https://www.klubhub.io/` redirects to the apex.

```sh
dig +short klubhub.io A
dig +short www.klubhub.io CNAME
curl -I https://klubhub.io/
```

GitHub Pages hosts only the informational site. A future hosted DJ application/API needs its own deployment and domain/subdomain; it must not be added to this static Pages artifact. Review GitHub Pages usage limits before adding commercial SaaS transactions or changing the site's purpose.

## Updating the site

Edit the HTML copy and CSS in `apps/site/public/`. Keep upcoming products and SaaS clearly marked until they actually launch. Update feature status against the application README/roadmap. Application token/font changes trigger a rebuild automatically. Keep `DESIGN.md` and the application stylesheet as visual references; the build reads the stylesheet, not the design document.

No application routes, API URLs, OAuth callbacks, repository license, or production service configuration are changed by this setup.

## Official references

- [Custom domains and current DNS targets](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/managing-a-custom-domain-for-your-github-pages-site)
- [Domain verification](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/verifying-your-custom-domain-for-github-pages)
- [Custom Pages workflows](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages)
- [Pages limits and permitted uses](https://docs.github.com/en/pages/getting-started-with-github-pages/github-pages-limits)
