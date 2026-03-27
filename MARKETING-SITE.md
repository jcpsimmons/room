# Marketing Site Handoff

The public-facing site lives in [`/marketing-site`](./marketing-site).

Use that directory directly for development, build, and deployment work:

```bash
cd marketing-site
bun install
bun run dev
```

Production build and local preview:

```bash
cd marketing-site
bun run build
bun run start
```

Deployment should target the `/marketing-site` app root, not the repository root. The Go CLI in the repo root is a separate instrument and does not need to be touched for website work.
