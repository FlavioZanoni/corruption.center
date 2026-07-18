FROM node:22-alpine AS deps
WORKDIR /app

# Enable pnpm via corepack
RUN corepack enable && corepack prepare pnpm@11.11.0 --activate

COPY package.json pnpm-lock.yaml* pnpm-workspace.yaml* ./
RUN pnpm install --frozen-lockfile

FROM node:22-alpine AS builder
WORKDIR /app

RUN corepack enable && corepack prepare pnpm@11.11.0 --activate

COPY --from=deps /app/node_modules ./node_modules
COPY . .

ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

# pnpm 11 runs a deps-status check before scripts and prompts for confirmation;
# there is no TTY in a docker build, so without CI=true `pnpm build` aborts
# (same workaround the dev compose applies to `pnpm dev`).
ENV CI=true
RUN pnpm build

FROM node:22-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production

RUN addgroup --system --gid 1001 nodejs && adduser --system --uid 1001 nextjs

# Enable pnpm (not strictly required at runtime, but safe)
RUN corepack enable && corepack prepare pnpm@11.11.0 --activate

# No COPY of public/: the directory is gone (icons and the OG card live in
# app/ as metadata file conventions). Restore the line if static assets return.
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
