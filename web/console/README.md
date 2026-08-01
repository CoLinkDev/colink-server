# CoLink Server Console

Web console for CoLink account and device management. This application is owned
and released together with `colink-server`.

**Tech stack:** Vue 3 · TypeScript · Vite · Tailwind CSS · Pinia · vue-router · vue-i18n · Radix Vue

## Development

```sh
pnpm install
pnpm dev        # Vite dev server; /api proxied to localhost:8080
```

## Build

```sh
pnpm build      # outputs to dist/; Docker embeds it into colink-server
pnpm preview    # preview production build locally
```

From the `colink-server` repository root, the equivalent commands are
`pnpm --dir web/console <command>`. The production build is served as static
files by `colink-server`.
